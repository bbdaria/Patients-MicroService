package main

import (
	"context"
	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	sf "github.com/sa-/slicefunk"
	"github.com/uptrace/bun"
	"time"
)

// PersonalId defines a schema of personal ids
type PersonalId struct {
	ID   string
	Type string
}

// EmergencyContact defines a schema of emergency contacts
type EmergencyContact struct {
	ID        int32 `bun:",pk,autoincrement"`
	Name      string
	Closeness string
	Phone     string
	PatientId int32
}

// Patient defines a schema of patients
type Patient struct {
	ID                int32 `bun:",pk,autoincrement"`
	Active            bool
	Name              string
	PersonalId        PersonalId `bun:"embed:personal_id_"`
	Gender            ppb.Patient_Gender
	PhoneNumber       string
	Languages         []string `bun:",array"`
	BirthDate         time.Time
	ReferredBy        string
	EmergencyContacts []*EmergencyContact `bun:"rel:has-many,join:id=patient_id"`
	SpecialNote       string
}

// toGRPC returns a GRPC version of PersonalId
func (personalId PersonalId) toGRPC() *ppb.Patient_PersonalId {
	return &ppb.Patient_PersonalId{
		Id:   personalId.ID,
		Type: personalId.Type,
	}
}

// toGRPC returns a GRPC version of EmergencyContact
func (contact EmergencyContact) toGRPC() *ppb.Patient_EmergencyContact {
	return &ppb.Patient_EmergencyContact{
		Name:      contact.Name,
		Closeness: contact.Closeness,
		Phone:     contact.Phone,
	}
}

// toGRPC returns a GRPC version of Patient
func (patient Patient) toGRPC() *ppb.Patient {
	EmergencyContacts := sf.Map(patient.EmergencyContacts,
		func(contact *EmergencyContact) *ppb.Patient_EmergencyContact { return contact.toGRPC() })
	return &ppb.Patient{
		Id:                patient.ID,
		Active:            patient.Active,
		Name:              patient.Name,
		PersonalId:        patient.PersonalId.toGRPC(),
		Gender:            patient.Gender,
		PhoneNumber:       patient.PhoneNumber,
		Languages:         patient.Languages,
		BirthDate:         patient.BirthDate.Format(time.DateOnly),
		Age:               int32(time.Now().Year() - patient.BirthDate.Year()),
		ReferredBy:        patient.ReferredBy,
		EmergencyContacts: EmergencyContacts,
		SpecialNote:       patient.SpecialNote,
	}
}

// createSchemaIfNotExists creates all required schemas for patient microservice
func createSchemaIfNotExists(ctx context.Context, db *bun.DB) error {
	models := []interface{}{
		(*Patient)(nil),
		(*EmergencyContact)(nil),
	}

	for _, model := range models {
		if _, err := db.NewCreateTable().IfNotExists().Model(model).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}
