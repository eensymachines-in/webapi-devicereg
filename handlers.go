package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/eensymachines-in/errx/httperr"
	"github.com/eensymachines-in/patio/aquacfg"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/mongo"
)

/* mongoClDBInCtx : utility function that can get mongo connection objects from the context if injected previously */
func mongoClDBInCtx(c *gin.Context) (*mongo.Client, *mongo.Database) {
	val, _ := c.Get("mongo-client")
	cl, _ := val.(*mongo.Client)
	val, _ = c.Get("mongo-database")
	db, _ := val.(*mongo.Database)
	return cl, db
}

// DeviceOfID : from the deivce of ID - objectid in the database or the mac id this can get the device details
// sets the device details in the context for the downstream handlers
func DeviceOfID(c *gin.Context) {
	cl, db := mongoClDBInCtx(c)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if cl == nil || db == nil {
		httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("failed to connect to mongo db")), log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
		if cl != nil {
			cl.Disconnect(ctx)
		}
		return
	}
	result := Device{}
	if err := DevicesCollc(db).GetOfId(DevMacID(c.Param("deviceid")), &result, ctx); err != nil {
		httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
		cl.Disconnect(ctx)
		return
	}
	c.Set("device", &result)
	c.Next()
}
func HndlOneDvc(c *gin.Context) {
	cl, db := mongoClDBInCtx(c)
	if cl == nil || db == nil {
		httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("failed to connect to mongo db")), log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
		return
	}
	defer cl.Disconnect(context.TODO()) // database connection closed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	val, _ := c.Get("device")
	deviceDetails, _ := val.(*Device)
	if c.Request.Method == "GET" {
		c.AbortWithStatusJSON(http.StatusOK, deviceDetails)
		return
	} else if c.Request.Method == "DELETE" {
		if err := DevicesCollc(db).DeleteDevice(c.Param("deviceid"), ctx); err != nil {
			httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
				"stack_trace": "HndlOneDvc/DELETE",
			}))
			return
		}
		c.AbortWithStatus(http.StatusOK)
		return
	} else if c.Request.Method == "PATCH" {
		path := c.Query("path")
		action := c.Query("action")
		if path == "config" {
			if action == "replace" {
				newCfg := aquacfg.Schedule{}
				if err := c.ShouldBind(&newCfg); err != nil {
					httperr.HttpErrOrOkDispatch(c, httperr.ErrBinding(err), log.WithFields(log.Fields{
						"stack_trace": "HndlLstDvcs/POST",
					}))
				}
				if err := DevicesCollc(db).PatchConfg(deviceDetails.MacID, newCfg, ctx); err != nil {
					httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
						"stack_trace": "HndlOneDvc/PATCH",
						"mac":         deviceDetails.MacID,
						"new_config":  newCfg.Config,
						"tickat":      newCfg.TickAt,
						"pulsegap":    newCfg.PulseGap,
						"interval":    newCfg.Interval,
					}))
					return
				}
				/* Rabbit publish the new config with routing key as mac id of the device
				Devices will bind queues using their own mac id for listening to messages
				since this is configs_direct amqp exchange only config changes will be posted here
				*/
				val, _ := c.Get("amqp-ch")
				amqpCh := val.(*amqp.Channel)
				val, _ = c.Get("amqp-conn")
				amqpConn := val.(*amqp.Connection)
				defer amqpConn.Close()
				defer amqpCh.Close()
				/* ------------ Setting up Publisher confirmations
				when the consumer (device on the ground acknowledges the receip[t of the msssage, this shall confirm here) */
				confirmations := amqpCh.NotifyPublish(make(chan amqp.Confirmation, 1))
				defer close(confirmations)

				/* ?Ready to publish the message to exchange */
				byt, _ := json.Marshal(newCfg)
				err := amqpCh.Publish(os.Getenv("AMQP_XNAME"), string(deviceDetails.MacID), false, false, amqp.Publishing{
					ContentType: "text/plain",
					Body:        byt,
				})
				/* Incase the publishing fails, the database changes will be out of sync from the device on the ground
				such cases its required to undo the changes on the database on our way out to the original configuration  */
				if err != nil {
					httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("failed to send message to amqp server %s", err)), log.WithFields(log.Fields{}))
					// since amqp publish  and db update should be atomic operation
					/* Unfortunately in the case if this fails, chances of which are minimal we still do get the device and the datbase out of sysnc
					hence you see no error is handled here */
					DevicesCollc(db).PatchConfg(deviceDetails.MacID, *deviceDetails.Cfg, ctx) // reverting the old settings
					return
				} else {
					// Success when publishing , wait for the confirmation acknowledgement
					select {
					case confrm := <-confirmations:
						if !confrm.Ack {
							httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("rejected by the rabbit mq server")), log.WithFields(log.Fields{
								"stack": "HndlOneDvc/PATCH",
							}))
							return
						}
						/* If received acknowledgement then does nothing escapes all the loopers to sending the device details with status ok */
					case <-time.After(8 * time.Second):
						/* Delay in getting the acknowledgement, deadline */
						httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("rabbitmq server timedout, no acknoedgement response")), log.WithFields(log.Fields{
							"stack": "HndlOneDvc/PATCH",
						}))
						return
					}
				}
			} else {
				c.AbortWithStatus(http.StatusMethodNotAllowed)
				return
			}
		} else if path == "users" {
			userEmails := []string{}
			if err := c.ShouldBind(&userEmails); err != nil {
				httperr.HttpErrOrOkDispatch(c, httperr.ErrBinding(err), log.WithFields(log.Fields{
					"stack_trace": "HndlOneDvc/PATCH",
				}))
				return
			}
			if action == "append" || action == "replace" {
				// append additional owners for the device
				if err := DevicesCollc(db).AppendUsers(deviceDetails.MacID, userEmails, map[string]bool{"append": false, "replace": true}[action], ctx); err != nil {
					httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
						"stack_trace": "HndlOneDvc/PATCH",
						"mac":         deviceDetails.MacID,
						"append":      userEmails,
					}))
					return
				}
			} else {
				// if the action is something else, this shall send out 405
				c.AbortWithStatus(http.StatusMethodNotAllowed)
				return
			}
		}
		// whenever done patching, getting the updated device details and dispatching via json  over http
		err := DevicesCollc(db).GetOfId(deviceDetails.MacID, deviceDetails, ctx)
		if err != nil {
			httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
				"stack_trace": "HndlOneDvc/PATCH/getting_updated",
			}))
			return
		}
		// time to dispatch the updated device details
		c.AbortWithStatusJSON(http.StatusOK, deviceDetails)
		return
	}
	c.AbortWithStatus(http.StatusMethodNotAllowed)
}

func HndlLstDvcs(c *gin.Context) {
	cl, db := mongoClDBInCtx(c)
	if cl == nil || db == nil {
		httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("failed to connect to mongo db")), log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
		return
	}
	defer cl.Disconnect(context.TODO()) // database connection closed
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if c.Request.Method == "POST" {
		/*
			Adding a new device registration
			- Binds the payload to Device
			- duplicate entrues not allowed, tracked by macID
			- invalid MAC IDs would be rejected
			- devices with no users are rejected
			- devices with invalid schedule configurations are rejected
			- ObjectID is auto generated and updated in the outgoing device
		*/
		newDevc := Device{}
		if err := c.ShouldBind(&newDevc); err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrBinding(err), log.WithFields(log.Fields{
				"stack_trace": "HndlLstDvcs/POST",
			}))
			return
		}
		if err := DevicesCollc(db).AddNewDevice(&newDevc, ctx); err != nil {
			httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
				"stack_trace": "HndlLstDvcs/POST",
			}))
			return
		}
		c.AbortWithStatusJSON(http.StatusOK, &newDevc)
		return
	} else if c.Request.Method == "GET" {
		// ?filter=users&user=email
		// val, is the email of the users for which we filter the devices
		filter := c.Query("filter")
		val := c.Query("user")
		if filter == "users" {
			result := []Device{}
			if err := DevicesCollc(db).DevicesOfUser(val, ctx, &result); err != nil {
				httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
					"stack_trace": "HndlLstDvcs/GET",
				}))
				return
			}
			c.AbortWithStatusJSON(http.StatusOK, result)
			return
		}
	}
	c.AbortWithStatus(http.StatusMethodNotAllowed)
}
