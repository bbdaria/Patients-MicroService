package main

import (
	"context"
	"database/sql"
	"fmt"
	ms "github.com/TekClinic/MicroService-Lib"
	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"net"
)

// patientsServer is an implementation of GRPC patient microservice. It provides access to database via db field
type patientsServer struct {
	ppb.UnimplementedPatientsServiceServer
	ms.BaseServiceServer
	db *bun.DB
}

const (
	envDBAddress     = "DB_ADDR"
	envDBUser        = "DB_USER"
	envDBDatabase    = "DB_DATABASE"
	envDBPassword    = "DB_PASSWORD"
	envBunDebugLevel = "BUN_DEBUG"

	applicationName = "patients"

	permissionDeniedMessage = "You don't have enough permission to access this resource"

	maxPaginationLimit = 50
)

// GetPatient returns a patient that corresponds to the given id
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned
// Requires admin role. If roles is not sufficient, codes.PermissionDenied is returned
// If patient with a given id doesn't exist, codes.NotFound is returned
func (server patientsServer) GetPatient(ctx context.Context, req *ppb.PatientRequest) (*ppb.Patient, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	patient := new(Patient)
	err = server.db.NewSelect().Model(patient).Where("? = ?", bun.Ident("id"), req.Id).Scan(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to fetch a user by id: %w", err).Error())
	}
	if patient == nil {
		return nil, status.Error(codes.NotFound, "User is not found")
	}
	return patient.toGRPC(), nil
}

// GetPatientsIds returns a list of patients' ids with given filters and pagination
// Requires authentication. If authentication is not valid, codes.Unauthenticated is returned
// Requires admin role. If roles is not sufficient, codes.PermissionDenied is returned
// Offset value is used for a pagination. Required be a non-negative value
// Limit value is used for a pagination. Required to be a positive value
func (server patientsServer) GetPatientsIds(ctx context.Context, req *ppb.PatientsRequest) (*ppb.PaginatedResponse, error) {
	claims, err := server.VerifyToken(ctx, req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}
	if !claims.HasRole("admin") {
		return nil, status.Error(codes.PermissionDenied, permissionDeniedMessage)
	}

	if req.Offset < 0 {
		return nil, status.Error(codes.InvalidArgument, "offset has to be a non-negative integer")
	}
	if req.Limit <= 0 {
		return nil, status.Error(codes.InvalidArgument, "offset has to be a positive integer")
	}
	if req.Limit > maxPaginationLimit {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("maximum allowed limit values is %d", maxPaginationLimit))
	}

	var ids []int32
	baseQuery := server.db.NewSelect().Model((*Patient)(nil)).Column("id")
	err = baseQuery.
		Offset(int(req.Offset)).
		Limit(int(req.Limit)).
		Scan(ctx, &ids)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to fetch users: %w", err).Error())
	}
	count, err := baseQuery.Count(ctx)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Errorf("failed to count users: %w", err).Error())
	}

	return &ppb.PaginatedResponse{
		Count:   int32(count),
		Results: ids,
	}, nil
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
		pgdriver.WithInsecure(true),
	)
	db := bun.NewDB(sql.OpenDB(connector), pgdialect.New())
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv(envBunDebugLevel),
	))
	return &patientsServer{BaseServiceServer: base, db: db}, nil
}

func main() {
	service, err := createPatientsServer()
	if err != nil {
		log.Fatal(err)
	}

	err = createSchemaIfNotExists(context.Background(), service.db)
	if err != nil {
		log.Fatal(err)
	}

	listen, err := net.Listen("tcp", ":"+service.GetPort())
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	ppb.RegisterPatientsServiceServer(srv, service)

	log.Println("Server listening on :" + service.GetPort())
	if err := srv.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
