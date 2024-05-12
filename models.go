package main

import (
	"regexp"

	"github.com/eensymachines-in/patio/aquacfg"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DevMacID string // mac ID of any device stored as string

func (mac DevMacID) IsValid() bool {
	// https://stackoverflow.com/questions/4260467/what-is-a-regular-expression-for-a-mac-address
	// delmiting character can be - or :
	r := regexp.MustCompile(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`)
	return r.MatchString(string(mac))
}

type Device struct {
	ID       primitive.ObjectID `bson:"_id,omitempty" json:"id,omitempty"`
	Name     string             `bson:"name" json:"name"` // its easier to use this when displaying on the front end
	MacID    DevMacID           `bson:"mac" json:"mac"`
	Location string             `bson:"location" json:"location"` // Google lat long coordinates as string
	Make     string             `bson:"make" json:"make"`         // string description of the platform hardware used
	Users    []string           `bson:"users" json:"users"`       // email list of user who can legit own  the device and thus control
	Cfg      *aquacfg.Schedule  `bson:"cfg" json:"cfg"`
}

// IsValid : validity of any device
/* macid is valid, atleast one user to control, configuration is not nil */
func (dev *Device) IsValid() bool {
	return dev.MacID.IsValid() && len(dev.Users) > 0 && dev.Cfg != nil && dev.Cfg.IsValid()
}

type CmdAck struct {
	DevcMacId string `json:"mac"` // device mac id
	Ack       bool   `json:"ack"`
}
