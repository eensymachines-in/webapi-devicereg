package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/eensymachines-in/errx/httperr"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// CORS : this allows all cross origin requests
func CORS(c *gin.Context) {
	// First, we add the headers with need to enable CORS
	// Make sure to adjust these headers to your needs
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Access-Control-Allow-Methods", "*")
	c.Header("Access-Control-Allow-Headers", "*")
	c.Header("Content-Type", "application/json")
	// Second, we handle the OPTIONS problem
	if c.Request.Method != "OPTIONS" {
		c.Next()
	} else {
		// Everytime we receive an OPTIONS request,
		// we just return an HTTP 200 Status Code
		// Like this, Angular can now do the real
		// request using any other method than OPTIONS
		c.AbortWithStatus(http.StatusOK)
	}
}

func RabbitConnectWithChn(connString, xname string) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := amqp.Dial(connString)
		if err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack":       "RabbitConnectWithChn",
				"conn_string": connString,
			}))
			return
		}
		// defer conn.Close()
		ch, err := conn.Channel()
		if err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack":  "RabbitConnectWithChn",
				"login":  os.Getenv("AMQP_LOGIN"),
				"server": os.Getenv("AMQP_SERVER"),
			}))
			conn.Close() // incase no channel, we close the channel before we exit the stack
			return
		}
		// NOTE: we shall be using a direct exchange with mac id specific routing key
		err = ch.ExchangeDeclare(
			xname,    // name
			"direct", // exhange type
			true,     // durable
			false,    //auto deleted
			false,    //internal
			false,    // nowait
			nil,      //amqp.table
		)
		if err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack":  "RabbitConnectWithChn",
				"login":  os.Getenv("AMQP_LOGIN"),
				"server": os.Getenv("AMQP_SERVER"),
			}))
			// incase declaring the exchange fails we close the channel and connection on our way out
			ch.Close()
			conn.Close()
			return
		}
		c.Set("amqp-ch", ch)
		c.Set("amqp-conn", conn)
		c.Next()
	}
}

func MongoConnectURI(uri, dbname string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil || client == nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack": "MongoConnect",
				"uri":   uri,
			}))
			return
		}
		c.Set("mongo-client", client)
		c.Set("mongo-database", client.Database(dbname))
	}
}
func MongoConnect(server, user, passwd, dbname string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(fmt.Sprintf("mongodb://%s:%s@%s:27017", user, passwd, server)))
		if err != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack":  "MongoConnect",
				"login":  user,
				"server": server,
			}))
			return
		}
		if client.Ping(ctx, readpref.Primary()) != nil {
			httperr.HttpErrOrOkDispatch(c, httperr.ErrGatewayConnect(err), log.WithFields(log.Fields{
				"stack":  "MongoConnect",
				"login":  user,
				"server": server,
			}))
			return
		}
		c.Set("mongo-client", client)
		c.Set("mongo-database", client.Database(dbname))
	}
}
