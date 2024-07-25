package main

import (
	"context"
	"time"

	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	sf "github.com/sa-/slicefunk"
	"github.com/uptrace/bun"
)

const birthDateFormat = "2006-01-02"

// PersonalID defines a schema of personal ids.
type PersonalID struct {
	ID   string
	Type string
}

// EmergencyContact defines a schema of emergency contacts.
type EmergencyContact struct {
	ID        int32  `bun:",pk,autoincrement"`
	Name      string `validate:"required,min=1,max=100"`
	Closeness string `validate:"required,min=1,max=100"`
	Phone     string `validate:"required,e164"`
	PatientID int32
}

// Patient defines a schema of patients.
type Patient struct {
	ID                int32               `bun:",pk,autoincrement" `
	Active            bool                ``
	Name              string              `validate:"required,min=1,max=100"`
	PersonalID        PersonalID          `bun:"embed:personal_id_" validate:"required"`
	Gender            ppb.Patient_Gender  ``
	PhoneNumber       string              `validate:"omitempty,e164"`
	Languages         []string            `bun:",array" validate:"max=10,dive,max=100"`
	BirthDate         time.Time           `validate:"required"`
	ReferredBy        string              `validate:"max=100"`
	EmergencyContacts []*EmergencyContact `bun:"rel:has-many,join:id=patient_id" validate:"max=10,dive"`
	SpecialNote       string              `validate:"max=500"`
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
		BirthDate:         patient.BirthDate.Format(birthDateFormat),
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

	// Postgres specific code. Add a text_searchable column for full-text search.
	_, err := db.NewRaw(
		"ALTER TABLE patients " +
			"ADD COLUMN IF NOT EXISTS text_searchable tsvector " +
			"GENERATED ALWAYS AS " +
			"(" +
			"setweight(to_tsvector('simple', coalesce(personal_id_id, '')), 'A') || " +
			"setweight(to_tsvector('simple', coalesce(phone_number, '')), 'A')   || " +
			"setweight(to_tsvector('simple', coalesce(name, '')), 'B')           || " +
			"setweight(to_tsvector('simple', coalesce(special_note, '')), 'C')   || " +
			"setweight(to_tsvector('simple', coalesce(referred_by, '')), 'D')" +
			") STORED").Exec(ctx)
	if err != nil {
		return err
	}

	/*
		SELECT id
		FROM patients,replace(
		    websearch_to_tsquery('simple', 'Jo 74')::text || ' ',
		    ''' ',
		    ''':*'
		  ) query
		WHERE text_searchable @@ query::tsquery
		ORDER BY ts_rank(text_searchable, query::tsquery) DESC;
	*/

	return nil
}
