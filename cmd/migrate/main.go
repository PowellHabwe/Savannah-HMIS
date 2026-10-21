package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/PowellHabwe/Savannah-HMIS/libs/config"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const dialect = "pgx"
var dir = flag.String("dir", "db/migrations", "directory with migration files")

func main() {
	flag.Usage = func() {
		fmt.Println("Usage: migrate [-dir DIR] COMMAND  (up, up-by-one, down, status, reset, version, redo)")
		flag.PrintDefaults()
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	c := config.New()
	db, err := goose.OpenDBWithDriver(dialect, c.DB.DSN())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := goose.RunContext(context.Background(), args[0], db, *dir, args[1:]...); err != nil {
		log.Fatalf("migrate %v: %v", args[0], err)
	}
}
