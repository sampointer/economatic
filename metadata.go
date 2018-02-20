package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/guregu/dynamo"
)

const schemaVersion = "20190219"

// MetaData represents run state
type MetaData struct {
	RunType string
	Version string // Key
}

// Save persists the run state
func (m MetaData) Save() error {
	db := dynamo.New(session.New())
	table := db.Table("economatic_metadata")

	m.Version = schemaVersion
	switch m.RunType {
	case "UP":
		m.RunType = "DOWN"
	case "DOWN":
		m.RunType = "UP"
	}

	err := table.Put(m).Run()
	return err
}

func getMetaData() (MetaData, error) {
	var result MetaData

	db := dynamo.New(session.New())
	table := db.Table("economatic_metadata")
	err := table.Get("Version", schemaVersion).One(&result)

	return result, err
}
