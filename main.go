/*
	For all the devices registered under eensymachines,
	or built by eensymachines this serves as the single source of truth for device status.
	Devices also maintain their configuration alongsided where in changing that configuration would mean the change is pushed to the device on the ground

author		: kneerunjun@gmail.com
Copyright 	: eensymachines.in@2024
*/
package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

const (
	/* Secrets inside the container are mounted at the location,
	check the deployment file for the name of the volume mounts and actual secret key literal for the path
	NOTE:  secret name in kubernetes  has no bearing here, volumename/literal-key*/
	MONGO_URI_SECRET = "/run/secrets/vol-mongouri/uri"
	AMQP_URI_SECRET  = "/run/secrets/vol-amqpuri/uri"
)

var (
	mongoConnectURI string = ""
	mongoDBName            = ""
	amqpConnectURI         = ""
	rabbitXchng            = "" // name of the rabbit queue
)

// readK8SecretMount : secrets mounted on the pod read inside the container
// fp		: filepath of the secret file
func readK8SecretMount(fp string) ([]string, error) {
	f, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	byt, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if bytes.HasSuffix(byt, []byte("\n")) {
		byt, _ = bytes.CutSuffix(byt, []byte("\n")) //often file read in will have this as a suffix
	}
	/* There could be multiple secrets in the same file separated by white space */
	return strings.Split(string(byt), " "), nil
}

// init : this will set logging parameters
// this will set mongo connection strings, database from env / secrets
// this will set amqp connection string from env / secrets
func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: false,
		PadLevelText:  true,
	})
	log.SetReportCaller(false)
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel) // default is info level, if verbose then trace
	val := os.Getenv("FLOG")
	if val == "1" {
		f, err := os.Open(os.Getenv("LOGF")) // file for logs
		if err != nil {
			log.SetOutput(os.Stdout) // error in opening log file
			log.Warn("Failed to open log file, log output set to stdout")
		}
		log.SetOutput(f) // log output set to file direction
		log.Infof("log output is set to file: %s", os.Getenv("LOGF"))

	} else {
		log.SetOutput(os.Stdout)
		log.Info("log output to stdout")
	}
	val = os.Getenv("SILENT")
	if val == "1" {
		log.SetLevel(log.ErrorLevel) // for production
	} else {
		log.SetLevel(log.DebugLevel) // for development
	}

	/* Making the mongo connection params  */
	secrets, err := readK8SecretMount(MONGO_URI_SECRET)
	if err != nil || len(secrets) == 0 {
		log.WithFields(log.Fields{
			"err":     err,
			"secrets": secrets,
		}).Fatalf("failed to read secret from mount")
	}
	mongoConnectURI = secrets[0]
	log.WithFields(log.Fields{
		"uri": mongoConnectURI,
	}).Debug("mongo connect uri from secret")

	/* attempting to ping the database before we start firing requests in
	if ofcourse each of the request will be using their own connection objects*/
	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoConnectURI))
	if err != nil || client == nil {
		log.Fatalf("failed database connection, %s", err)
	}
	err = client.Ping(ctx, readpref.Primary())
	if err != nil {
		log.Fatalf("failed to ping database, %s", err)
	}
	log.Info("database is reachable..")
	defer client.Disconnect(ctx) // purpose of this connection is served

	mongoDBName = os.Getenv("MONGO_DB_NAME")
	if mongoDBName == "" {
		log.Fatal("invalid/empty name for mongo db, cannot proceed")
	}

	/* Making AMQP connection.. */
	secrets, err = readK8SecretMount(AMQP_URI_SECRET)
	if err != nil || len(secrets) == 0 {
		log.WithFields(log.Fields{
			"err":     err,
			"secrets": secrets,
		}).Fatalf("failed to read secret from mount")
	}
	amqpConnectURI = secrets[0]
	log.WithFields(log.Fields{
		"uri": amqpConnectURI,
	}).Debug("amqp connect uri from secret")
	rabbitXchng = os.Getenv("AMQP_XNAME")

}

func main() {
	log.Info("Starting webapi-devicereg..")
	defer log.Warn("Closing webapi-devicereg")
	gin.SetMode(gin.DebugMode)
	r := gin.Default()

	devices := r.Group("/api/devices").Use(CORS).Use(MongoConnectURI(mongoConnectURI, mongoDBName))

	// Posting a new device registrations
	// Getting a list of devices filtered on a field
	devices.POST("", HndlLstDvcs)
	devices.GET("", HndlLstDvcs) //?filter=users&user=userid

	// Getting a single device details , either on mac or mongo oid
	devices.GET("/:deviceid", DeviceOfID, HndlOneDvc)
	// Patching device details  - config or users
	// ?path=users&action=append
	// ?path=config
	devices.PATCH("/:deviceid", RabbitConnectWithChn(amqpConnectURI, rabbitXchng), DeviceOfID, HndlOneDvc)
	// Removing a device registration completely
	devices.DELETE("/:deviceid", HndlOneDvc)

	log.Fatal(r.Run(":8080"))
}
