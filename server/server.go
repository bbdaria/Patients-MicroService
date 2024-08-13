package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/go-playground/validator/v10"
	sf "github.com/sa-/slicefunk"

	ms "github.com/TekClinic/MicroService-Lib"
	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// patientsServer is an implementation of GRPC patient microservice. It provides access to a database via db field.
type patientsServer struct {
	ppb.UnimplementedPatientsServiceServer
	ms.BaseServiceServer
	db *bun.DB
	// use a single instance of Validate, it caches struct info
	validate *validator.Validate
}

const (
	envDBAddress  = "DB_ADDR"
	envDBUser     = "DB_USER"
	envDBDatabase = "DB_DATABASE"
	envDBPassword = "DB_PASSWORD"

	applicationName = "patients"

	permissionDeniedMessage = "You don't have enough permission to access this resource"

	maxPaginationLimit = 50
)

// GetPatient returns a patient that corresponds to the given id.
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned.
// Requires an admin role. If roles are not sufficient, codes.PermissionDenied is returned.
// If a patient with a given id doesn't exist, codes.NotFound is returned.
func (server patientsServer) GetPatient(ctx context.Context, req *ppb.GetPatientRequest) (
	*ppb.GetPatientResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	patient := new(Patient)
	err = server.db.NewSelect().
		Model(patient).
		Relation("EmergencyContacts").
		Where("? = ?", bun.Ident("id"), req.GetId()).
		Scan(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "patient is not found")
		}
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to fetch a patients by id: %w", err).Error())
	}
	return &ppb.GetPatientResponse{Patient: patient.toGRPC()}, nil
}

// GetPatientsIDs returns a list of patients' ids with given filters and pagination.
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned.
// Requires an admin role. If roles are not sufficient, codes.PermissionDenied is returned.
// Offset value is used for pagination. Required be a non-negative value.
// Limit value is used for pagination. Required to be a positive value.
func (server patientsServer) GetPatientsIDs(ctx context.Context,
	req *ppb.GetPatientsIDsRequest) (*ppb.GetPatientsIDsResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	if req.GetOffset() < 0 {
		return nil, status.Error(codes.InvalidArgument, "offset has to be a non-negative integer")
	}
	if req.GetLimit() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "limit has to be a positive integer")
	}
	if req.GetLimit() > maxPaginationLimit {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("maximum allowed limit values is %d", maxPaginationLimit))
	}

	var ids []int32
	baseQuery := server.db.NewSelect().Model((*Patient)(nil)).Column("id")

	if req.GetSearch() != "" {
		// Postgres specific code. Use full-text search to search for patients.
		baseQuery = baseQuery.
			TableExpr("replace(websearch_to_tsquery('simple', ?)::text || ' ',''' ',''':*') query", req.GetSearch()).
			Where("text_searchable @@ query::tsquery", req.GetSearch()).
			OrderExpr("ts_rank(text_searchable, query::tsquery) DESC")
	}

	err = baseQuery.
		Offset(int(req.GetOffset())).
		Limit(int(req.GetLimit())).
		Scan(ctx, &ids)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to fetch patients: %w", err).Error())
	}
	count, err := baseQuery.Count(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to count patients: %w", err).Error())
	}

	return &ppb.GetPatientsIDsResponse{
		Count:   int32(count),
		Results: ids,
	}, nil
}

// CreatePatient creates a patient with the given specifications.
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned.
// Requires an admin role. If roles are not sufficient, codes.PermissionDenied is returned.
// If some argument is missing or not valid, codes.InvalidArgument is returned.
func (server patientsServer) CreatePatient(ctx context.Context,
	req *ppb.CreatePatientRequest) (*ppb.CreatePatientResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	birthDate, err := time.Parse(birthDateFormat, req.GetBirthDate())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument,
			fmt.Errorf("failed to parse birth date: %w", err).Error())
	}

	patient := Patient{
		Active: true,
		Name:   req.GetName(),
		PersonalID: PersonalID{
			ID:   req.GetPersonalId().GetId(),
			Type: req.GetPersonalId().GetType(),
		},
		Gender:      req.GetGender(),
		PhoneNumber: req.GetPhoneNumber(),
		Languages:   req.GetLanguages(),
		BirthDate:   birthDate,
		ReferredBy:  req.GetReferredBy(),
		EmergencyContacts: sf.Map(req.GetEmergencyContacts(),
			func(contact *ppb.Patient_EmergencyContact) *EmergencyContact {
				return &EmergencyContact{
					Name:      contact.GetName(),
					Closeness: contact.GetCloseness(),
					Phone:     contact.GetPhone(),
				}
			}),
		SpecialNote: req.GetSpecialNote(),
	}
	if err = server.validate.Struct(patient); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err = server.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// firstly, insert the patient itself
		if _, txErr := tx.NewInsert().Model(&patient).Exec(ctx); txErr != nil {
			return txErr
		}
		// afterward, insert all its emergence contacts
		for _, contact := range patient.EmergencyContacts {
			contact.PatientID = patient.ID

			if _, txErr := tx.NewInsert().Model(contact).Exec(ctx); txErr != nil {
				return txErr
			}
		}
		return nil
	}); err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to create a patient: %w", err).Error())
	}
	return &ppb.CreatePatientResponse{Id: patient.ID}, nil
}

// DeletePatient deletes a patient with the given id.
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned.
// Requires an admin role. If roles are not sufficient, codes.PermissionDenied is returned.
// If a patient with a given id doesn't exist, codes.NotFound is returned.
func (server patientsServer) DeletePatient(ctx context.Context, req *ppb.DeletePatientRequest) (
	*ppb.DeletePatientResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	res, err := server.db.NewDelete().Model((*Patient)(nil)).Where("id = ?", req.GetId()).Exec(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to delete a patient: %w", err).Error())
	}
	// if db supports affected rows count and no rows were affected, return not found
	rows, err := res.RowsAffected()
	if err == nil && rows == 0 {
		return nil, status.Error(codes.NotFound, "patient is not found")
	}
	return &ppb.DeletePatientResponse{}, nil
}

// UpdatePatient updates a patient with the given id and data.
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned.
// Requires an admin role. If roles are not sufficient, codes.PermissionDenied is returned.
// If some argument is missing or not valid, codes.InvalidArgument is returned.
// If a patient with a given id doesn't exist, codes.NotFound is returned.
func (server patientsServer) UpdatePatient(ctx context.Context, req *ppb.UpdatePatientRequest) (
	*ppb.UpdatePatientResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	patient, err := patientFromGRPC(req.GetPatient())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err = server.validate.Struct(patient); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if patient.ID == 0 {
		return nil, status.Error(codes.InvalidArgument, "Patient ID is required")
	}

	if err = server.db.RunInTx(ctx, &sql.TxOptions{}, func(ctx context.Context, tx bun.Tx) error {
		// firstly, update the patient itself
		res, txErr := tx.NewUpdate().Model(&patient).WherePK().Exec(ctx)
		if txErr != nil {
			return status.Error(codes.Internal, fmt.Errorf("failed to update a patient: %w", txErr).Error())
		}

		// if db supports affected rows count and no rows were affected, return not found
		rows, rowsErr := res.RowsAffected()
		if rowsErr == nil && rows == 0 {
			return status.Error(codes.NotFound, "patient is not found")
		}

		// afterward, delete all its emergence contacts
		_, txErr = tx.NewDelete().Model((*EmergencyContact)(nil)).Where("patient_id = ?", patient.ID).Exec(ctx)
		if txErr != nil {
			return status.Error(codes.Internal, fmt.Errorf("failed to delete emergency contacts: %w", txErr).Error())
		}

		// finally, insert all its emergence contacts
		for _, contact := range patient.EmergencyContacts {
			contact.PatientID = patient.ID

			if _, txErr = tx.NewInsert().Model(contact).Exec(ctx); txErr != nil {
				return txErr
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return &ppb.UpdatePatientResponse{Id: patient.ID}, nil
}

// createPatientsServer initializes a patientsServer with all the necessary fields.
func createPatientsServer() (*patientsServer, error) {
	base, err := ms.CreateBaseServiceServer()
	if err != nil {
		return nil, err
	}
	addr, err := ms.GetRequiredEnv(envDBAddress)
	if err != nil {
		return nil, err
	}
	user, err := ms.GetRequiredEnv(envDBUser)
	if err != nil {
		return nil, err
	}
	password, err := ms.GetRequiredEnv(envDBPassword)
	if err != nil {
		return nil, err
	}
	database, err := ms.GetRequiredEnv(envDBDatabase)
	if err != nil {
		return nil, err
	}
	connector := pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(addr),
		pgdriver.WithUser(user),
		pgdriver.WithPassword(password),
		pgdriver.WithDatabase(database),
		pgdriver.WithApplicationName(applicationName),
		pgdriver.WithInsecure(!ms.HasSecureConnection()),
	)
	db := bun.NewDB(sql.OpenDB(connector), pgdialect.New())
	db.AddQueryHook(ms.GetDBQueryHook())
	return &patientsServer{
		BaseServiceServer: base,
		db:                db,
		validate:          validator.New(validator.WithRequiredStructEnabled())}, nil
}

func main() {
	service, err := createPatientsServer()
	if err != nil {
		zap.L().Fatal("Failed to create a patient server", zap.Error(err))
	}

	err = createSchemaIfNotExists(context.Background(), service.db)
	if err != nil {
		zap.L().Fatal("Failed to create a schema", zap.Error(err))
	}

	listen, err := net.Listen("tcp", ":"+service.GetPort())
	if err != nil {
		zap.L().Fatal("Failed to listen", zap.Error(err))
	}

	srv := grpc.NewServer(ms.GetGRPCServerOptions()...)
	ppb.RegisterPatientsServiceServer(srv, service)

	zap.L().Info("Server listening on :" + service.GetPort())
	if err = srv.Serve(listen); err != nil {
		zap.L().Fatal("Failed to serve", zap.Error(err))
	}
}
