package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	authpb "github.com/TekClinic/Auth-MicroService/auth_protobuf"
	ppb "github.com/TekClinic/Patients-MicroService/patients_protobuf"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/extra/bundebug"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"os"
	"strings"
)

type Service struct {
	host string
	port string
}

func (s Service) getAddr() string {
	return s.host + ":" + s.port
}

type Patient struct {
	bun.BaseModel `bun:"table:patients,alias:p"`

	UserId string `bun:"type:uuid,pk"`
	Name   string
}

type patientsServer struct {
	ppb.UnimplementedPatientsServiceServer
	db   *bun.DB
	port string
	auth *Service
}

func (server patientsServer) GetPatient(ctx context.Context, req *ppb.PatientRequest) (*ppb.Patient, error) {
	if req.GetUserId() == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id argument is missing")
	}
	// for now only patient can fetch its own data
	requesterId, err := server.getUserId(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	if requesterId != req.GetUserId() {
		return nil, status.Error(codes.PermissionDenied, "Only patient can fetch its own data")
	}

	patient, err := server.fetchPatientById(ctx, req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &ppb.Patient{UserId: patient.UserId, Name: patient.Name}, nil
}

func (server patientsServer) GetMe(ctx context.Context, req *ppb.MeRequest) (*ppb.Patient, error) {
	requesterId, err := server.getUserId(ctx, req.GetToken())
	if err != nil {
		return nil, err
	}
	patient, err := server.fetchPatientById(ctx, requesterId)
	if err != nil {
		return nil, err
	}
	return &ppb.Patient{UserId: patient.UserId, Name: patient.Name}, nil
}

func (server patientsServer) fetchPatientById(ctx context.Context, userId string) (*Patient, error) {
	patient := new(Patient)
	err := server.db.NewSelect().Model(patient).Where("user_id = ?", userId).Scan(ctx)
	if err != nil {
		return nil, status.Error(codes.NotFound, "")
	}
	return patient, nil
}

func (server patientsServer) getUserId(ctx context.Context, token string) (string, error) {
	conn, err := grpc.Dial(server.auth.getAddr(),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Print(err)
		return "", status.Error(codes.Internal, err.Error())
	}
	defer conn.Close()
	client := authpb.NewAuthServiceClient(conn)
	clientResponse, err := client.ValidateToken(ctx, &authpb.TokenRequest{Token: token})
	return clientResponse.GetUserId(), err
}

func getRequiredEnv(key string) (string, error) {
	value, set := os.LookupEnv(key)
	if !set {
		return "", errors.New(key + " environment variable is missing")
	}
	return value, nil
}

func getOptionalEnv(key string, def string) string {
	value, set := os.LookupEnv(key)
	if set {
		return value
	}
	return def
}

func fetchServiceParameters(serviceName string) (*Service, error) {
	host, err := getRequiredEnv(fmt.Sprintf("MS_%s_HOST", strings.ToUpper(serviceName)))
	if err != nil {
		return nil, err
	}

	port := getOptionalEnv(fmt.Sprintf("MS_%s_PORT", strings.ToUpper(serviceName)), "9090")
	return &Service{host: host, port: port}, nil
}

func fetchServerParameters() (*patientsServer, error) {
	addr, err := getRequiredEnv("DB_ADDR")
	if err != nil {
		return nil, err
	}
	user, err := getRequiredEnv("DB_USER")
	if err != nil {
		return nil, err
	}
	password, err := getRequiredEnv("DB_PASSWORD")
	if err != nil {
		return nil, err
	}
	database, err := getRequiredEnv("DB_DATABASE")
	if err != nil {
		return nil, err
	}
	port := getOptionalEnv("GRPC_PORT", "9090")
	auth, err := fetchServiceParameters("auth")
	if err != nil {
		return nil, err
	}
	connector := pgdriver.NewConnector(
		pgdriver.WithNetwork("tcp"),
		pgdriver.WithAddr(addr),
		pgdriver.WithUser(user),
		pgdriver.WithPassword(password),
		pgdriver.WithDatabase(database),
		pgdriver.WithApplicationName("patients"),
		pgdriver.WithInsecure(true),
	)
	db := bun.NewDB(sql.OpenDB(connector), pgdialect.New())
	db.AddQueryHook(bundebug.NewQueryHook(
		bundebug.WithVerbose(true),
		bundebug.FromEnv("BUNDEBUG"),
	))
	return &patientsServer{db: db, port: port, auth: auth}, nil
}

func main() {
	service, err := fetchServerParameters()
	if err != nil {
		log.Fatal(err)
	}

	_, err = service.db.NewCreateTable().IfNotExists().Model((*Patient)(nil)).Exec(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	listen, err := net.Listen("tcp", ":"+service.port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	ppb.RegisterPatientsServiceServer(srv, service)

	log.Println("Server listening on :" + service.port)
	if err := srv.Serve(listen); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
