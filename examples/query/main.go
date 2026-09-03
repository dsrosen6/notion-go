// Command query filters and sorts the rows of a data source.
//
// Set NOTION_TOKEN to an integration token, and NOTION_DATABASE_ID to a
// database the integration has been shared with.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	notion "github.com/dsrosen6/notion-go"
)

func main() {
	token, databaseID := os.Getenv("NOTION_TOKEN"), os.Getenv("NOTION_DATABASE_ID")
	if token == "" || databaseID == "" {
		log.Fatal("set NOTION_TOKEN and NOTION_DATABASE_ID")
	}

	ctx := context.Background()
	client := notion.NewClient(token)

	// Queries address a data source, not the database that contains it.
	db, err := client.Databases.Retrieve(ctx, notion.ExtractID(databaseID))
	if err != nil {
		var apiErr *notion.APIError
		if errors.As(err, &apiErr) && apiErr.Code == notion.CodeObjectNotFound {
			log.Fatal("no such database, or the integration has not been shared with it")
		}
		log.Fatalf("retrieving the database: %v", err)
	}
	if len(db.DataSources) == 0 {
		log.Fatal("the database has no data sources")
	}
	dataSourceID := db.DataSources[0].ID

	ds, err := client.DataSources.Retrieve(ctx, dataSourceID)
	if err != nil {
		log.Fatalf("retrieving the data source: %v", err)
	}
	fmt.Printf("%s — columns:\n", ds.Title.PlainText())
	for name, schema := range ds.Properties {
		fmt.Printf("  %-20s %s\n", name, schema.SchemaType())
	}

	// Rows edited in the past month, newest first.
	params := notion.QueryParams{
		Filter: notion.ByLastEditedTime().PastMonth(),
		Sorts:  []notion.Sort{notion.SortByLastEditedTime(notion.Descending)},
	}

	fmt.Println("\nRecently edited:")
	count := 0
	for page, err := range client.DataSources.QueryAll(ctx, dataSourceID, params) {
		if err != nil {
			log.Fatalf("querying: %v", err)
		}
		count++
		fmt.Printf("  %s\n", page.Title())
		if count == 20 {
			break // The iterator stops fetching as soon as we stop reading.
		}
	}
	if count == 0 {
		fmt.Println("  (nothing edited in the past month)")
	}
}
