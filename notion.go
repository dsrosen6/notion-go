// Package notion is a client for the Notion API.
//
// It is a Go port of the official JavaScript SDK, @notionhq/client, and matches
// its behavior — retry policy, error taxonomy, pagination semantics, request
// encoding — while presenting an idiomatic Go surface.
//
// Start with [NewClient]:
//
//	client := notion.NewClient(os.Getenv("NOTION_TOKEN"))
//	page, err := client.Pages.Retrieve(ctx, pageID)
//
// Values the API models as tagged unions — blocks, rich text, property values,
// property schemas — are Go interfaces implemented by one struct per variant,
// so they are consumed with a type switch:
//
//	switch b := block.(type) {
//	case *notion.ParagraphBlock:
//		fmt.Println(b.Paragraph.RichText.PlainText())
//	case *notion.Heading1Block:
//		fmt.Println(b.Heading1.RichText.PlainText())
//	}
//
// Notion adds new variants over time. Anything this package does not recognize
// decodes into an Unknown variant ([UnknownBlock] and friends) that retains the
// raw JSON, so decoding never fails on a workspace using a newer feature.
package notion

// Version is the version of this package, reported in the User-Agent header.
const Version = "0.1.0"

// DefaultNotionVersion is the Notion-Version header sent with every request
// unless overridden with [WithNotionVersion]. It matches the version pinned by
// @notionhq/client v5.26.0.
const DefaultNotionVersion = "2025-09-03"

// DefaultBaseURL is the root of the Notion API. Requests go to
// DefaultBaseURL + "/v1/" + path.
const DefaultBaseURL = "https://api.notion.com"

// EmptyObject is a payload carrying no fields. It marshals to {} rather than
// null, which the API requires for the tagged-union variants that have no data
// of their own — dividers, breadcrumbs, and existence filters among them.
type EmptyObject struct{}
