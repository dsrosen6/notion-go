package notion

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"net/http"
)

// maxNestingDepth is how deep children may be nested in one request. Notion
// accepts a block, its children, and their children, and rejects anything
// deeper (BlockObjectRequest -> BlockObjectWithSingleLevelOfChildrenRequest
// -> BlockObjectRequestWithoutChildren, common.ts:1810, 2049, 2525). A column
// list is the exception: its columns are structural, so the blocks inside a
// column may nest one level more (ColumnListRequest -> ColumnWithChildrenRequest
// -> BlockObjectWithSingleLevelOfChildrenRequest, common.ts:2207-2212).
const maxNestingDepth = 3

// ErrNestingTooDeep reports children nested deeper than one request allows.
// Append the deeper levels in follow-up calls instead.
var ErrNestingTooDeep = errors.New("notion: blocks nested more than three levels deep in one request")

// ErrInvalidNesting reports a column_list or column block where the API does
// not accept one. A column_list may appear only at the top level of a request
// and may hold only columns; a column may appear at the top level, when
// appending to an existing column_list, or directly inside a column_list
// (common.ts:1838-1839, 2207-2212).
var ErrInvalidNesting = errors.New("notion: column_list or column block placed where the API does not accept it")

// BlocksService reads and edits page content. Reach it through
// [Client.Blocks].
type BlocksService struct {
	c *Client
}

// Retrieve returns a single block by ID.
//
// It does not return the block's children even when HasChildren is true; read
// those with [BlocksService.Children].
func (s *BlocksService) Retrieve(ctx context.Context, blockID string, opts ...RequestOption) (Block, error) {
	var raw jsonBlock
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "blocks/" + escapeID(blockID),
	}, &raw, opts...)
	if err != nil {
		return nil, err
	}
	return raw.Block, nil
}

// Children returns one page of a block's or page's direct children.
//
// Most callers want [BlocksService.AllChildren] instead.
func (s *BlocksService) Children(ctx context.Context, blockID string, params PageParams, opts ...RequestOption) (*List[Block], error) {
	var out blockList
	err := s.c.do(ctx, request{
		method: http.MethodGet,
		path:   "blocks/" + escapeID(blockID) + "/children",
		query:  params.query(),
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return &List[Block]{
		Object:        out.Object,
		Results:       out.Results,
		NextCursor:    out.NextCursor,
		HasMore:       out.HasMore,
		RequestStatus: out.RequestStatus,
	}, nil
}

// AllChildren iterates every direct child of a block or page, fetching pages
// as needed.
//
// It does not descend: a child with HasChildren set must be read separately.
//
//	for block, err := range client.Blocks.AllChildren(ctx, pageID) {
//		if err != nil {
//			return err
//		}
//		fmt.Println(block.BlockType())
//	}
func (s *BlocksService) AllChildren(ctx context.Context, blockID string, opts ...RequestOption) iter.Seq2[Block, error] {
	return paginate(ctx, "", func(ctx context.Context, cursor string) ([]Block, string, error) {
		page, err := s.Children(ctx, blockID, PageParams{StartCursor: cursor, PageSize: pageSizeMax}, opts...)
		if err != nil {
			return nil, "", err
		}
		return page.Results, page.NextCursor, nil
	})
}

// AppendChildren adds blocks to the end of a block's or page's children and
// returns the blocks as created.
//
// Notion accepts at most 100 blocks per call, nested at most three levels deep.
// Append deeper levels in follow-up calls addressed at the newly created
// blocks.
func (s *BlocksService) AppendChildren(ctx context.Context, blockID string, children []Block, opts ...RequestOption) ([]Block, error) {
	if err := validateNesting(children, 1); err != nil {
		return nil, err
	}

	body := struct {
		Children BlockList `json:"children"`
	}{Children: children}

	var out blockList
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "blocks/" + escapeID(blockID) + "/children",
		body:   body,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return out.Results, nil
}

// AppendChildrenAfter inserts blocks directly after the block with the given
// ID, rather than at the end.
//
// The result begins with the inserted blocks and continues with every sibling
// that now follows them, which is how Notion answers a positioned append.
func (s *BlocksService) AppendChildrenAfter(ctx context.Context, blockID, afterID string, children []Block, opts ...RequestOption) ([]Block, error) {
	if err := validateNesting(children, 1); err != nil {
		return nil, err
	}

	body := struct {
		Children BlockList `json:"children"`
		After    string    `json:"after,omitempty"`
	}{Children: children, After: afterID}

	var out blockList
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "blocks/" + escapeID(blockID) + "/children",
		body:   body,
	}, &out, opts...)
	if err != nil {
		return nil, err
	}
	return out.Results, nil
}

// Update replaces a block's content. Pass a block of the same type as the one
// being updated, carrying only the fields to change.
//
//	client.Blocks.Update(ctx, blockID, notion.NewParagraph("Revised text."))
func (s *BlocksService) Update(ctx context.Context, blockID string, block Block, opts ...RequestOption) (Block, error) {
	var raw jsonBlock
	err := s.c.do(ctx, request{
		method: http.MethodPatch,
		path:   "blocks/" + escapeID(blockID),
		body:   block,
	}, &raw, opts...)
	if err != nil {
		return nil, err
	}
	return raw.Block, nil
}

// Delete moves a block to the trash, along with everything nested inside it.
func (s *BlocksService) Delete(ctx context.Context, blockID string, opts ...RequestOption) (Block, error) {
	var raw jsonBlock
	err := s.c.do(ctx, request{
		method: http.MethodDelete,
		path:   "blocks/" + escapeID(blockID),
	}, &raw, opts...)
	if err != nil {
		return nil, err
	}
	return raw.Block, nil
}

// jsonBlock lets an endpoint decode into the Block interface, which cannot
// decode itself.
type jsonBlock struct {
	Block Block
}

func (j *jsonBlock) UnmarshalJSON(data []byte) error {
	block, err := DecodeBlock(data)
	if err != nil {
		return err
	}
	j.Block = block
	return nil
}

// blockList mirrors List[Block] with a concrete slice type, since the generic
// List cannot name BlockList's custom decoder.
type blockList struct {
	Object        string         `json:"object"`
	Results       BlockList      `json:"results"`
	NextCursor    string         `json:"next_cursor"`
	HasMore       bool           `json:"has_more"`
	RequestStatus *RequestStatus `json:"request_status,omitempty"`
}

// validateNesting reports children nested deeper than the API accepts in one
// request, or column blocks placed where it does not accept them. Notion's
// type definitions encode these rules across four parallel unions; Go cannot
// express them in types, so they are checked before sending. depth is the
// level of blocks themselves, 1 for the top of the request.
func validateNesting(blocks []Block, depth int) error {
	return checkNesting(blocks, depth, maxNestingDepth, nil)
}

// checkNesting walks one level of blocks. limit is the deepest level allowed
// beneath this point, and parent is the block holding blocks, nil at the top.
func checkNesting(blocks []Block, depth, limit int, parent Block) error {
	if len(blocks) == 0 {
		return nil
	}
	if depth > limit {
		return fmt.Errorf("%w: found a block at depth %d", ErrNestingTooDeep, depth)
	}
	_, inColumnList := parent.(*ColumnListBlock)
	for _, block := range blocks {
		childLimit := limit
		switch block.(type) {
		case *ColumnListBlock:
			if depth != 1 {
				return fmt.Errorf("%w: column_list at depth %d, want it at the top level", ErrInvalidNesting, depth)
			}
			childLimit = limit + 1
		case *ColumnBlock:
			if depth != 1 && !inColumnList {
				return fmt.Errorf("%w: column at depth %d is not inside a column_list", ErrInvalidNesting, depth)
			}
		default:
			if inColumnList {
				return fmt.Errorf("%w: %s inside a column_list, want only columns", ErrInvalidNesting, block.BlockType())
			}
		}
		if err := checkNesting(childrenOf(block), depth+1, childLimit, block); err != nil {
			return err
		}
	}
	return nil
}

// childrenOf returns the blocks nested inside one block in a request, or nil
// for block types that cannot nest.
func childrenOf(block Block) []Block {
	switch b := block.(type) {
	case *ParagraphBlock:
		return b.Paragraph.Children
	case *Heading1Block:
		return b.Heading1.Children
	case *Heading2Block:
		return b.Heading2.Children
	case *Heading3Block:
		return b.Heading3.Children
	case *Heading4Block:
		return b.Heading4.Children
	case *BulletedListItemBlock:
		return b.BulletedListItem.Children
	case *NumberedListItemBlock:
		return b.NumberedListItem.Children
	case *QuoteBlock:
		return b.Quote.Children
	case *ToDoBlock:
		return b.ToDo.Children
	case *ToggleBlock:
		return b.Toggle.Children
	case *TemplateBlock:
		return b.Template.Children
	case *CalloutBlock:
		return b.Callout.Children
	case *SyncedBlock:
		return b.SyncedBlock.Children
	case *ColumnListBlock:
		return b.ColumnList.Children
	case *ColumnBlock:
		return b.Column.Children
	case *TableBlock:
		return b.Table.Children
	case *TabBlock:
		return b.Tab.Children
	default:
		return nil
	}
}
