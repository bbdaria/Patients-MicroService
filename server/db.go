package main

import (
	"context"
	"time"

	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	sf "github.com/sa-/slicefunk"
	"github.com/uptrace/bun"
)

// PersonalID defines a schema of personal ids.
type PersonalID struct {
	ID   string
	Type string
}

// EmergencyContact defines a schema of emergency contacts.
type EmergencyContact struct {
	ID        int32 `bun:",pk,autoincrement"`
	Name      string
	Closeness string
	Phone     string
	PatientID int32
}

// Patient defines a schema of patients.
type Patient struct {
	ID                int32 `bun:",pk,autoincrement"`
	Active            bool
	Name              string
	PersonalID        PersonalID `bun:"embed:personal_id_"`
	Gender            ppb.Patient_Gender
	PhoneNumber       string
	Languages         []string `bun:",array"`
	BirthDate         time.Time
	ReferredBy        string
	EmergencyContacts []*EmergencyContact `bun:"rel:has-many,join:id=patient_id"`
	SpecialNote       string
}

// toGRPC returns a GRPC version of PersonalID.
func (personalId PersonalID) toGRPC() *ppb.Patient_PersonalID {
	return &ppb.Patient_PersonalID{
		Id:   personalId.ID,
		Type: personalId.Type,
	}
}

// toGRPC returns a GRPC version of EmergencyContact.
func (contact EmergencyContact) toGRPC() *ppb.Patient_EmergencyContact {
	return &ppb.Patient_EmergencyContact{
		Name:      contact.Name,
		Closeness: contact.Closeness,
		Phone:     contact.Phone,
	}
}

// toGRPC returns a GRPC version of Patient.
func (patient Patient) toGRPC() *ppb.Patient {
	emergencyContacts := sf.Map(patient.EmergencyContacts,
		func(contact *EmergencyContact) *ppb.Patient_EmergencyContact { return contact.toGRPC() })
	return &ppb.Patient{
		Id:                patient.ID,
		Active:            patient.Active,
		Name:              patient.Name,
		PersonalId:        patient.PersonalID.toGRPC(),
		Gender:            patient.Gender,
		PhoneNumber:       patient.PhoneNumber,
		Languages:         patient.Languages,
		BirthDate:         patient.BirthDate.Format("2006-01-02"),
		Age:               int32(time.Now().Year() - patient.BirthDate.Year()),
		ReferredBy:        patient.ReferredBy,
		EmergencyContacts: emergencyContacts,
		SpecialNote:       patient.SpecialNote,
	}
}

// createSchemaIfNotExists creates all required schemas for patient microservice.
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
