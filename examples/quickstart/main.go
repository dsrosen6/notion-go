// Command quickstart creates a page with some content and reads it back.
//
// Set NOTION_TOKEN to an integration token, and NOTION_PARENT_PAGE_ID to a page
// the integration has been shared with.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	notion "github.com/dsrosen6/notion-go"
)

func main() {
	token, parentID := os.Getenv("NOTION_TOKEN"), os.Getenv("NOTION_PARENT_PAGE_ID")
	if token == "" || parentID == "" {
		log.Fatal("set NOTION_TOKEN and NOTION_PARENT_PAGE_ID")
	}

	ctx := context.Background()
	client := notion.NewClient(token)

	// A page URL works as well as a bare ID.
	parentID = notion.ExtractID(parentID)

	page, err := client.Pages.Create(ctx, notion.CreatePageParams{
		Parent:     notion.Parent{Type: notion.ParentTypePage, PageID: parentID},
		Icon:       notion.NewEmojiIcon("🚀"),
		Properties: notion.PropertyValues{"title": notion.NewTitle("Created from Go")},
		Children: notion.BlockList{
			notion.NewHeading1("Overview"),
			notion.NewParagraph("This page was created by the notion-go quickstart."),
			notion.NewToDo("Read the docs", true),
			notion.NewToDo("Ship something", false),
			notion.NewDivider(),
			notion.NewCode("fmt.Println(\"hello\")", "go"),
		},
	})
	if err != nil {
		log.Fatalf("creating the page: %v", err)
	}
	fmt.Printf("Created %q at %s\n\n", page.Title(), page.URL)

	fmt.Println("Content:")
	for block, err := range client.Blocks.AllChildren(ctx, page.ID) {
		if err != nil {
			log.Fatalf("reading blocks: %v", err)
		}
		fmt.Printf("  %-12s %s\n", block.BlockType(), describe(block))
	}
}

// describe renders a one-line summary of a block.
func describe(block notion.Block) string {
	switch b := block.(type) {
	case *notion.ParagraphBlock:
		return b.Paragraph.RichText.PlainText()
	case *notion.Heading1Block:
		return b.Heading1.RichText.PlainText()
	case *notion.ToDoBlock:
		mark := " "
		if b.ToDo.IsChecked() {
			mark = "x"
		}
		return fmt.Sprintf("[%s] %s", mark, b.ToDo.RichText.PlainText())
	case *notion.CodeBlock:
		return fmt.Sprintf("(%s) %s", b.Code.Language, b.Code.RichText.PlainText())
	default:
		return ""
	}
}
