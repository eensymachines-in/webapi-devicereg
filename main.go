/*
	For all the devices registered under eensymachines,
	or built by eensymachines this serves as the single source of truth for device status
	Here you can patch devices for their running configuration, such as controlling relays thru clock

author		: kneerunjun@gmail.com
Copyright 	: eensymachines.in@2024
*/
package main

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
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
	if bytes.HasSuffix(byt, []byte("\n")) { //often file read in will have this as a suffix
		byt, _ = bytes.CutSuffix(byt, []byte("\n"))
	}
	/* There could be multiple secrets in the same file separated by white space */
	return strings.Split(string(byt), " "), nil
}

func init() {
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: false,
		PadLevelText:  true,
	})
	log.SetReportCaller(false)
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel) // default is info level, if verbose then trace
	val := os.Getenv("FLOG")
	log.WithFields(log.Fields{
		"flog": os.Getenv("FLOG"),
	}).Debug("environment variable")
	if val == "1" {
		f, err := os.Open(os.Getenv("LOGF")) // file for logs
		if err != nil {
			log.SetOutput(os.Stdout) // error in opening log file
			log.Debug("log output is Stdout")
		}
		log.SetOutput(f) // log output set to file direction
		log.Debug("log output is set to file")

	} else {
		log.SetOutput(os.Stdout)
		log.Debug("log output is Stdout")
	}
	val = os.Getenv("SILENT")
	log.WithFields(log.Fields{
		"silence": os.Getenv("SILENT"),
	}).Debug("environment variable")
	if val == "1" {
		log.SetLevel(log.ErrorLevel) // for development
	} else {
		log.SetLevel(log.DebugLevel) // for production
	}

	log.Debug("Making database connections ..")

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

	mongoDBName = os.Getenv("MONGO_DB_NAME")
	if mongoDBName == "" {
		log.Fatal("invalid/empty name for mongo db, cannot proceed")
	}

	log.Debug("Making rabbitmq connections ..")
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
