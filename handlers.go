package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/eensymachines-in/errx/httperr"
	"github.com/eensymachines-in/patio/aquacfg"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
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
	if cl == nil || db == nil {
		httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(fmt.Errorf("failed to connect to mongo db")), log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
		return
	}
	// NOTE: will not close the client here, since this is not the last of all the handlers in the chain
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := Device{}
	if err := DevicesCollc(db).GetOfId(DevMacID(c.Param("deviceid")), &result, ctx); err != nil {
		httperr.HttpErrOrOkDispatch(c, err, log.WithFields(log.Fields{
			"stack_trace": "HndlLstDvcs",
			"client_null": cl == nil,
			"db_null":     db == nil,
		}))
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
		// Posting a new device as registration
		newDevc := Device{}
		if err := c.ShouldBind(&newDevc); err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrBinding(err), log.WithFields(log.Fields{
				"stack_trace": "HndlLstDvcs/POST",
			}))
			return
		}
		log.WithFields(log.Fields{
			"mac": newDevc.MacID,
		}).Debug("payload bound")
		// Will add the new device registration to the database
		// Send back the device details with OID details on the same object
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
		filter := c.Query("filter")
		val := c.Query("user")
		if filter == "users" {
			// val, is the email of the users for which we filter the devices
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
