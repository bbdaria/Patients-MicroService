package main

import (
	"context"
	"database/sql"
	"errors"
	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"os"
)

type Patient struct {
	bun.BaseModel `bun:"table:patients,alias:p"`

	UserId string `bun:"type:uuid,pk"`
	Name   string
}

type patientsServer struct {
	ppb.UnimplementedPatientsServiceServer
	db *bun.DB
}

func (server patientsServer) GetPatient(ctx context.Context, req *ppb.PatientRequest) (*ppb.Patient, error) {
	patient := new(Patient)
	err := server.db.NewSelect().Model(patient).Where("user_id = ?", req.GetUserId()).Scan(ctx)
	if err != nil {
		return nil, status.Error(codes.NotFound, "")
	}
	return &ppb.Patient{UserId: patient.UserId, Name: patient.Name}, nil
}

func createConnector() (*pgdriver.Connector, error) {
	addr, set := os.LookupEnv("DB_ADDR")
	if !set {
		return nil, errors.New("DB_ADDR environment variable is missing")
	}
	user, set := os.LookupEnv("DB_USER")
	if !set {
		return nil, errors.New("DB_USER environment variable is missing")
	}
	password, set := os.LookupEnv("DB_PASSWORD")
	if !set {
		return nil, errors.New("DB_PASSWORD environment variable is missing")
	}
	database, set := os.LookupEnv("DB_DATABASE")
	if !set {
		return nil, errors.New("DB_DATABASE environment variable is missing")
	}
	return pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(addr),
		pgdriver.WithUser(user),
		pgdriver.WithPassword(password),
		pgdriver.WithDatabase(database),
		pgdriver.WithApplicationName("patients"),
		pgdriver.WithInsecure(true),
	), nil
}

func main() {
	connector, err := createConnector()
	if err != nil {
		log.Fatal(err)
	}

	db := bun.NewDB(sql.OpenDB(connector), pgdialect.New())
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))
	db.NewCreateTable().IfNotExists().Model((*Patient)(nil)).Exec(context.Background())

	srv := grpc.NewServer()
	ppb.RegisterPatientsServiceServer(srv, &patientsServer{db: db})

	// Register reflection service on gRPC authServer.
	reflection.Register(srv)

	grpcPort, set := os.LookupEnv("GRPC_PORT")
	if !set {
		grpcPort = "9090"
	}
	listen, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	log.Println("Server listening on :" + grpcPort)
	if err := srv.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
