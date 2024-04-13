package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/eensymachines-in/errx/httperr"
	"github.com/eensymachines-in/patio/aquacfg"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/mgo.v2/bson"
)

var (
	DevicesCollc = func(db *mongo.Database) QueryDevices {
		return &qryDevices{Collection: db.Collection("devices")}
	}
)

type QueryDevices interface {
	GetOfId(mac DevMacID, result *Device, ctx context.Context) httperr.HttpErr
	AddNewDevice(*Device, context.Context) httperr.HttpErr
	DeleteDevice(mac string, ctx context.Context) httperr.HttpErr
	DevicesOfUser(userid string, ctx context.Context, result *[]Device) httperr.HttpErr
	PatchConfg(DevMacID, aquacfg.Schedule, context.Context) httperr.HttpErr
	AppendUsers(DevMacID, []string, bool, context.Context) httperr.HttpErr
}

type qryDevices struct {
	*mongo.Collection
}

// GetOfId : Gets a single device of Mac ID
// result is out param that will get hydrated with the details.
// Errors in case  document not found, or the query fails.
func (qd *qryDevices) GetOfId(mac DevMacID, result *Device, ctx context.Context) httperr.HttpErr {
	*result = Device{} // fresh instance for the output
	sr := qd.FindOne(ctx, bson.M{"mac": mac})
	if sr.Err() != nil {
		if errors.Is(sr.Err(), mongo.ErrNoDocuments) {
			return httperr.ErrResourceNotFound(fmt.Errorf("device not found %s", mac))
		} else {
			return httperr.ErrDBQuery(sr.Err())
		}
	}
	if err := sr.Decode(result); err != nil {
		return httperr.ErrBinding(err)
	}
	return nil
}

// AddNewDevice : Adds a new device to the collection of devices
// Will validate the device before adding, error if invalidated - includes the validation for mac, cfg, users
// Once the device is added, mongo object id is updated on the device - json marshalling and dispatch
func (qd *qryDevices) AddNewDevice(dev *Device, ctx context.Context) httperr.HttpErr {
	if !dev.IsValid() || dev == nil {
		return httperr.ErrInvalidParam(fmt.Errorf("one or more fields on the device is invalid"))
	}
	count, err := qd.CountDocuments(ctx, bson.M{"mac": dev.MacID})
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	if count > 0 {
		return httperr.DuplicateResourceErr(fmt.Errorf("device with Mac %s already registered", dev.MacID))
	}
	sr, err := qd.InsertOne(ctx, dev)
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	dev.ID = sr.InsertedID.(primitive.ObjectID)
	return nil
}

// DeleteDevice: permanently deletes the device registration
// Error when mac id isnt valid
// Does NOTcheck for if the mac id exists .. silently deletes the data
// Once deleted data cannot be recovered.
func (qd *qryDevices) DeleteDevice(mac string, ctx context.Context) httperr.HttpErr {
	if !DevMacID(mac).IsValid() {
		return httperr.ErrInvalidParam(fmt.Errorf("invalid MAC for the device to delete %s", mac))
	}
	_, err := qd.DeleteOne(ctx, bson.M{"mac": mac})
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	return nil
}

// DevicesOfUser: Gets all the devices for the given email id fo the user
// result		: list of devices, wiped clean before planting results into it
func (qd *qryDevices) DevicesOfUser(userid string, ctx context.Context, result *[]Device) httperr.HttpErr {
	if userid == "" {
		// BUG: We arent checking for email id pattern of the user
		return httperr.ErrInvalidParam(fmt.Errorf("invalid user email as owner of the device %s", userid))
	}
	*result = []Device{} // instantiating a fresh slice
	cursor, err := qd.Find(ctx, bson.M{"users": bson.M{"$elemMatch": bson.M{"$eq": userid}}})
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	err = cursor.All(ctx, result)
	if err != nil {
		return httperr.ErrBinding(err)
	}
	return nil
}

// PatchConfg : Pacthes the schedule for the device, given the mac id, new schedule
// Error when mac id invalid or the query fails
func (qd *qryDevices) PatchConfg(mac DevMacID, sched aquacfg.Schedule, ctx context.Context) httperr.HttpErr {
	if !mac.IsValid() {
		return httperr.ErrInvalidParam(fmt.Errorf("invalid mac id %s for the device being patched", mac))
	}
	if !sched.IsValid() {
		return httperr.ErrInvalidParam(fmt.Errorf("invalid schedule for the device. Check schedule fields for rule violation"))
	}
	_, err := qd.UpdateOne(ctx, bson.M{"mac": mac}, bson.M{"$set": bson.M{"cfg": sched}})
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	return nil
}

// AppendUsers:  patches (appends / replaces ) the list of legit users
func (qd *qryDevices) AppendUsers(mac DevMacID, users []string, replace bool, ctx context.Context) httperr.HttpErr {
	if !mac.IsValid() {
		return httperr.ErrInvalidParam(fmt.Errorf("invalid mac id %s for the device being patched", mac))
	}
	var patch bson.M
	if !replace {
		patch = bson.M{"$addToSet": bson.M{"users": bson.M{"$each": users}}}
	} else {
		patch = bson.M{"$set": bson.M{"users": users}}
	}
	_, err := qd.UpdateOne(ctx, bson.M{"mac": mac}, patch)
	if err != nil {
		return httperr.ErrDBQuery(err)
	}
	return nil
}
