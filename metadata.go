package main

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/guregu/dynamo"
)

const schema_version = "20190219"

type MetaData struct {
	RunType string
	Version string // Key
}

func (m MetaData) Save() error {
	db := dynamo.New(session.New())
	table := db.Table("economatic_metadata")

	m.Version = schema_version
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
	err := table.Get("Version", schema_version).One(&result)

	return result, err
}
