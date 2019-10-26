package main

import (
	"log"
	"time"

	"github.com/0x13a/golang.cafe/pkg/config"
	"github.com/0x13a/golang.cafe/pkg/database"
)

func main() {
	log.Println("cleaning up expired sponsored ads")
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("unable to load config %v", err)
	}
	conn, err := database.GetDbConn(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("unable to connect to postgres: %v", err)
	}
	defer database.CloseDbConn(conn)
	log.Printf("attempting to demote expired sponsored 30days pinned job ads\n")
	affected, err := database.DemoteJobAdsOlderThan(conn, time.Now().AddDate(0, 0, -30), database.JobAdSponsoredPinnedFor30Days)
	if err != nil {
		log.Fatalf("unable to demote expired sponsored 30days pinned job ads %v", err)
	}
	log.Printf("demoted %d expired sponsored 30days pinned job ads\n", affected)

	log.Printf("attempting to demote expired sponsored 7days pinned job ads\n")
	affected, err = database.DemoteJobAdsOlderThan(conn, time.Now().AddDate(0, 0, -7), database.JobAdSponsoredPinnedFor7Days)
	if err != nil {
		log.Fatalf("unable to demote expired sponsored 7days pinned job ads %v", err)
	}
	log.Printf("demoted %d expired sponsored 7days pinned job ads\n", affected)

	log.Printf("attempting to demote expired highlighted 30days job ads\n")
	affected, err = database.DemoteJobAdsOlderThan(conn, time.Now().AddDate(0, 0, -30), database.JobAdSponsoredBackground)
	if err != nil {
		log.Fatalf("unable to demote expired highlighted 30days job ads %v", err)
	}
	log.Printf("demoted %d expired highlighted 30days job ads\n", affected)

	log.Printf("also cleaning up expired apply tokens")
	err = database.CleanupExpiredApplyTokens(conn)
	if err != nil {
		log.Fatalf("unable to cleanup expired apply tokens err %v", err)
	}
	log.Printf("finished to cleanup expired apply tokens")
}
