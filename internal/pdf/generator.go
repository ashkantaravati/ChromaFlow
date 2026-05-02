package pdf

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type Generator struct {
	timeout     time.Duration
	chromeWSURL string
}

func NewGenerator(timeoutSec int, chromeWSURL string) *Generator {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}

	return &Generator{
		timeout:     time.Duration(timeoutSec) * time.Second,
		chromeWSURL: chromeWSURL,
	}
}

func (g *Generator) GeneratePDF(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	controlURL, cleanup, err := g.browserControlURL(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	browser := rod.New().Context(ctx).ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	defer func() { _ = browser.Close() }()

	page, err := browser.Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("open page: %w", err)
	}
	page = page.Context(ctx)
	defer func() { _ = page.Close() }()

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait for page load: %w", err)
	}

	stream, err := page.PDF(&proto.PagePrintToPDF{
		Landscape:         false,
		PrintBackground:   true,
		PreferCSSPageSize: false,
	})
	if err != nil {
		return nil, fmt.Errorf("print pdf: %w", err)
	}
	defer func() { _ = stream.Close() }()

	pdf, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("read pdf stream: %w", err)
	}

	return pdf, nil
}

func (g *Generator) browserControlURL(ctx context.Context) (string, func(), error) {
	if g.chromeWSURL != "" {
		return g.chromeWSURL, func() {}, nil
	}

	chromePath := os.Getenv("CHROME_BIN")
	if chromePath == "" {
		chromePath, _ = launcher.LookPath()
	}
	if chromePath == "" {
		chromePath = "/usr/bin/chromium-browser"
	}

	l := launcher.New().
		Context(ctx).
		Bin(chromePath).
		Headless(true).
		NoSandbox(true)

	controlURL, err := l.Launch()
	if err != nil {
		return "", func() {}, fmt.Errorf("launch browser: %w", err)
	}

	return controlURL, l.Kill, nil
}
