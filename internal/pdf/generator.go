package pdf

import (
	"context"
	"io"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type Generator struct {
	timeout time.Duration
}

func NewGenerator(timeoutSec int) *Generator {
	return &Generator{
		timeout: time.Duration(timeoutSec) * time.Second,
	}
}

func (g *Generator) GeneratePDF(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	browser := rod.New().Context(ctx).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage(url)
	defer page.MustClose()

	page.MustWaitLoad()

	stream, err := page.PDF(&proto.PagePrintToPDF{
		PrintBackground: true,
	})
	if err != nil {
		return nil, err
	}

	// Read the stream into bytes
	pdf, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}

	return pdf, nil
}
