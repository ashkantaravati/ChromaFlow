package pdf

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type Generator struct {
	timeout     time.Duration
	chromeWSURL string
	mu          sync.Mutex
	browserInst *rod.Browser
	cleanup     func()
}

func NewGenerator(timeoutSec int, chromeWSURL string) *Generator {
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	return &Generator{timeout: time.Duration(timeoutSec) * time.Second, chromeWSURL: chromeWSURL}
}

func (g *Generator) GeneratePDF(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	browser, err := g.ensureBrowser(ctx)
	if err != nil {
		return nil, err
	}

	page, err := browser.Context(ctx).Page(proto.TargetCreateTarget{URL: url})
	if err != nil {
		return nil, fmt.Errorf("open page: %w", err)
	}
	page = page.Context(ctx)
	defer func() { _ = page.Close() }()

	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait for page load: %w", err)
	}

	stream, err := page.PDF(&proto.PagePrintToPDF{Landscape: false, PrintBackground: true, PreferCSSPageSize: false})
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

func (g *Generator) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserInst != nil {
		_ = g.browserInst.Close()
		g.browserInst = nil
	}
	if g.cleanup != nil {
		g.cleanup()
		g.cleanup = nil
	}
}

func (g *Generator) ensureBrowser(ctx context.Context) (*rod.Browser, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.browserInst != nil {
		return g.browserInst, nil
	}
	controlURL, cleanup, err := g.browserControlURL(ctx)
	if err != nil {
		return nil, err
	}
	browser := rod.New().Context(ctx).ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		cleanup()
		return nil, fmt.Errorf("connect browser: %w", err)
	}
	g.browserInst = browser
	g.cleanup = cleanup
	return g.browserInst, nil
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
	l := launcher.New().Context(ctx).Bin(chromePath).Headless(true).NoSandbox(true)
	controlURL, err := l.Launch()
	if err != nil {
		return "", func() {}, fmt.Errorf("launch browser: %w", err)
	}
	return controlURL, l.Kill, nil
}
