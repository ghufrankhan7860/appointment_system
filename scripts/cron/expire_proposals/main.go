// Command expire_proposals closes reschedule proposals nobody answered in
// time, as if the patient side declined them, and exits. Run it every few
// minutes from a scheduler. A proposal answered meanwhile is skipped, so it is
// safe to overlap with itself and with patients answering.
//
//	go run ./scripts/cron/expire_proposals
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"
	_ "time/tzdata"

	"appointment_system/config"
	"appointment_system/internal/apps/appointment"
	"appointment_system/internal/apps/appointment/repositories"
	"appointment_system/internal/apps/appointment/services"
	"appointment_system/internal/platform/datastore"
)

const (
	expireBatchSize = 100
	expireTimeout   = time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Printf("expire reschedule proposals: %v", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), expireTimeout)
	defer cancel()

	store, err := datastore.Open(ctx, datastore.Config{
		Host:            config.Database.Host,
		Port:            config.Database.Port,
		User:            config.Database.User,
		Password:        config.Database.Password,
		Database:        config.Database.Name,
		SSLMode:         config.Database.SSLMode,
		MaxOpenConns:    config.Database.MaxOpenConns,
		MaxIdleConns:    config.Database.MaxIdleConns,
		ConnMaxLifetime: config.Database.ConnMaxLifetime,
		ConnectTimeout:  config.Database.ConnectTimeout,
	})
	if err != nil {
		return fmt.Errorf("initialize datastore: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close datastore: %v", err)
		}
	}()

	appointmentRepository, err := repositories.NewAppointmentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize appointment repository: %w", err)
	}
	outboxRepository, err := repositories.NewOutboxRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize outbox repository: %w", err)
	}
	userPatientRepository, err := repositories.NewUserPatientRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize user-patient repository: %w", err)
	}
	doctorRepository, err := repositories.NewDoctorRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize doctor repository: %w", err)
	}
	availabilityRepository, err := repositories.NewAvailabilityRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize availability repository: %w", err)
	}
	paymentRepository, err := repositories.NewPaymentRepository(store.DB)
	if err != nil {
		return fmt.Errorf("initialize payment repository: %w", err)
	}
	txManager, err := repositories.NewGORMTransactionManager(store.DB)
	if err != nil {
		return fmt.Errorf("initialize transaction manager: %w", err)
	}
	paymentGateway, err := appointment.NewPaymentGateway(appointment.PaymentSettings{
		Provider:              config.Payment.Provider,
		RazorpayKeyID:         config.Payment.RazorpayKeyID,
		RazorpayKeySecret:     config.Payment.RazorpayKeySecret,
		RazorpayWebhookSecret: config.Payment.RazorpayWebhookSecret,
	})
	if err != nil {
		return fmt.Errorf("initialize payment gateway: %w", err)
	}

	availabilityService, err := services.NewAvailabilityService(
		doctorRepository,
		availabilityRepository,
		appointmentRepository,
		txManager,
		config.App.Location,
		time.Now,
	)
	if err != nil {
		return fmt.Errorf("initialize availability service: %w", err)
	}
	paymentService, err := services.NewPaymentService(
		paymentRepository,
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		paymentGateway,
		txManager,
		time.Now,
	)
	if err != nil {
		return fmt.Errorf("initialize payment service: %w", err)
	}
	appointmentService, err := services.NewAppointmentService(
		appointmentRepository,
		outboxRepository,
		userPatientRepository,
		doctorRepository,
		availabilityService,
		paymentService,
		txManager,
	)
	if err != nil {
		return fmt.Errorf("initialize appointment service: %w", err)
	}

	expired, err := appointmentService.ExpireProposals(ctx, expireBatchSize)
	if err != nil {
		return err
	}

	log.Printf("expired %d reschedule proposals", expired)
	return nil
}
