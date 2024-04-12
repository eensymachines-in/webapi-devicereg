/*
	For all the devices registered under eensymachines,
	or built by eensymachines this serves as the single source of truth for device status

author		: kneerunjun@gmail.com
Copyright 	: eensymachines.in@2024
*/
package main

import (
	"io"
	"os"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	MONGO_URI_SECRET = "/run/secrets/mongo_uri"
)

var (
	mongoConnectURI string = ""
	mongoDBName            = ""
)

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
			log.Debug("log output is Stdout")
		}
		log.SetOutput(f) // log output set to file direction
		log.Debug("log output is set to file")

	} else {
		log.SetOutput(os.Stdout)
		log.Debug("log output is Stdout")
	}
	val = os.Getenv("SILENT")
	if val == "1" {
		log.SetLevel(log.ErrorLevel) // for development
	} else {
		log.SetLevel(log.DebugLevel) // for production
	}

	log.Debug("Making database connections ..")
	f, err := os.Open(MONGO_URI_SECRET)
	if err != nil || f == nil {
		log.Fatalf("failed to open mongo connection uri from secret file %s", err)
	}
	byt, err := io.ReadAll(f)
	if err != nil {
		log.Fatalf("failed to read mongo connection uri from secret file %s", err)
	}
	mongoConnectURI = string(byt) // now that we have mongo connect uri we shall be
	if mongoConnectURI == "" {
		log.Fatal("mongo connect uri is empty, check secret file and rerun application")
	}
	mongoDBName = os.Getenv("MONGO_DB_NAME")
	if mongoDBName == "" {
		log.Fatal("invalid/empty name for mongo db, cannot proceed")
	}
}

func main() {
	log.Info("Starting webapi-devicereg..")
	defer log.Warn("Closing webapi-devicereg")
	gin.SetMode(gin.DebugMode)
	r := gin.Default()

	devices := r.Group("/api/devices")
	devices.Use(CORS).Use(MongoConnectURI(mongoConnectURI, mongoDBName))

	// Posting a new device registrations
	// Getting a list of devices filtered on a field
	devices.POST("", HndlLstDvcs)
	devices.GET("", HndlLstDvcs) //?filter=users&user=userid

	// Getting a single device details , either on mac or mongo oid
	devices.GET("/:deviceid", DeviceOfID, HndlOneDvc)
	// Patching device details  - config or users
	// ?path=users&action=append
	// ?path=config
	devices.PATCH("/:deviceid", DeviceOfID, HndlOneDvc)
	// Removing a device registration completely
	devices.DELETE("/:deviceid", HndlOneDvc)

	log.Fatal(r.Run(":8080"))
}
