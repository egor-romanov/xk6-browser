package tests

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"image/png"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type emulateMediaOpts struct {
	Media         string `js:"media"`
	ColorScheme   string `js:"colorScheme"`
	ReducedMotion string `js:"reducedMotion"`
}

type jsFrameBaseOpts struct {
	Timeout string
	Strict  bool
}

const sampleHTML = `<div><b>Test</b><ol><li><i>One</i></li></ol></div>`

func TestPageEmulateMedia(t *testing.T) {
	tb := newTestBrowser(t)
	p := tb.NewPage(nil)

	p.EmulateMedia(tb.toGojaValue(emulateMediaOpts{
		Media:         "print",
		ColorScheme:   "dark",
		ReducedMotion: "reduce",
	}))

	result := p.Evaluate(tb.toGojaValue("() => matchMedia('print').matches"))
	res, ok := result.(goja.Value)
	require.True(t, ok)
	assert.True(t, res.ToBoolean(), "expected media 'print'")

	result = p.Evaluate(tb.toGojaValue("() => matchMedia('(prefers-color-scheme: dark)').matches"))
	res, ok = result.(goja.Value)
	require.True(t, ok)
	assert.True(t, res.ToBoolean(), "expected color scheme 'dark'")

	result = p.Evaluate(tb.toGojaValue("() => matchMedia('(prefers-reduced-motion: reduce)').matches"))
	res, ok = result.(goja.Value)
	require.True(t, ok)
	assert.True(t, res.ToBoolean(), "expected reduced motion setting to be 'reduce'")
}

func TestPageContent(t *testing.T) {
	t.Parallel()

	tb := newTestBrowser(t)
	p := tb.NewPage(nil)

	content := `<!DOCTYPE html><html><head></head><body><h1>Hello</h1></body></html>`
	p.SetContent(content, nil)

	assert.Equal(t, content, p.Content())
}

func TestPageEvaluate(t *testing.T) {
	t.Parallel()

	t.Run("ok/func_arg", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)

		got := p.Evaluate(
			tb.toGojaValue("(v) => { window.v = v; return window.v }"),
			tb.toGojaValue("test"),
		)

		require.IsType(t, tb.toGojaValue(""), got)
		gotVal, ok := got.(goja.Value)
		require.True(t, ok)
		assert.Equal(t, "test", gotVal.Export())
	})

	t.Run("err", func(t *testing.T) {
		t.Parallel()

		testCases := []struct {
			name, js, errMsg string
		}{
			{
				"promise",
				`async () => { return await new Promise((res, rej) => { rej('rejected'); }); }`,
				"evaluating JS: rejected",
			},
			{
				"syntax", `() => {`,
				"evaluating JS: SyntaxError: Unexpected token ')'",
			},
			{"undef", "undef", "evaluating JS: ReferenceError: undef is not defined"},
		}

		for _, tc := range testCases {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				tb := newTestBrowser(t)
				assertExceptionContains(t, tb.runtime(), func() {
					p := tb.NewPage(nil)
					p.Evaluate(tb.toGojaValue(tc.js))
				}, tc.errMsg)
			})
		}
	})
}

func TestPageGoto(t *testing.T) {
	b := newTestBrowser(t, withFileServer())
	p := b.NewPage(nil)

	url := b.staticURL("empty.html")
	r, err := p.Goto(url, nil)
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, url, r.URL(), `expected URL to be %q, result of navigation was %q`, url, r.URL())
}

func TestPageGotoDataURI(t *testing.T) {
	b := newTestBrowser(t)
	p := b.NewPage(nil)

	r, err := p.Goto("data:text/html,hello", nil)
	require.NoError(t, err)
	assert.Nil(t, r, `expected response to be nil`)
	require.NoError(t, err)
}

func TestPageGotoWaitUntilLoad(t *testing.T) {
	b := newTestBrowser(t, withFileServer())
	p := b.NewPage(nil)

	opts := b.toGojaValue(struct {
		WaitUntil string `js:"waitUntil"`
	}{
		WaitUntil: "load",
	})
	_, err := p.Goto(b.staticURL("wait_until.html"), opts)
	require.NoError(t, err)
	var (
		results = p.Evaluate(b.toGojaValue("() => window.results"))
		actual  []string
	)
	_ = b.runtime().ExportTo(b.asGojaValue(results), &actual)

	assert.EqualValues(t,
		[]string{"DOMContentLoaded", "load"}, actual,
		`expected "load" event to have fired`,
	)
}

func TestPageGotoWaitUntilDOMContentLoaded(t *testing.T) {
	b := newTestBrowser(t, withFileServer())
	p := b.NewPage(nil)

	opts := b.toGojaValue(struct {
		WaitUntil string `js:"waitUntil"`
	}{
		WaitUntil: "domcontentloaded",
	})
	_, err := p.Goto(b.staticURL("wait_until.html"), opts)
	require.NoError(t, err)
	var (
		results = p.Evaluate(b.toGojaValue("() => window.results"))
		actual  []string
	)
	_ = b.runtime().ExportTo(b.asGojaValue(results), &actual)

	assert.EqualValues(t,
		"DOMContentLoaded", actual[0],
		`expected "DOMContentLoaded" event to have fired`,
	)
}

func TestPageInnerHTML(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		p := newTestBrowser(t).NewPage(nil)
		p.SetContent(sampleHTML, nil)
		assert.Equal(t, `<b>Test</b><ol><li><i>One</i></li></ol>`, p.InnerHTML("div", nil))
	})

	t.Run("err_empty_selector", func(t *testing.T) {
		t.Parallel()

		defer func() {
		}()

		tb := newTestBrowser(t)
		assertExceptionContains(t, tb.runtime(), func() {
			p := tb.NewPage(nil)
			p.InnerHTML("", nil)
		}, "The provided selector is empty")
	})

	t.Run("err_wrong_selector", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		p.SetContent(sampleHTML, nil)
		require.Panics(t, func() { p.InnerHTML("p", tb.toGojaValue(jsFrameBaseOpts{Timeout: "100"})) })
	})
}

func TestPageInnerText(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		p := newTestBrowser(t).NewPage(nil)
		p.SetContent(sampleHTML, nil)
		assert.Equal(t, "Test\nOne", p.InnerText("div", nil))
	})

	t.Run("err_empty_selector", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		assertExceptionContains(t, tb.runtime(), func() {
			p := tb.NewPage(nil)
			p.InnerText("", nil)
		}, "The provided selector is empty")
	})

	t.Run("err_wrong_selector", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		p.SetContent(sampleHTML, nil)
		require.Panics(t, func() { p.InnerText("p", tb.toGojaValue(jsFrameBaseOpts{Timeout: "100"})) })
	})
}

func TestPageTextContent(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		p := newTestBrowser(t).NewPage(nil)
		p.SetContent(sampleHTML, nil)
		assert.Equal(t, "TestOne", p.TextContent("div", nil))
	})

	t.Run("err_empty_selector", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		assertExceptionContains(t, tb.runtime(), func() {
			p := tb.NewPage(nil)
			p.TextContent("", nil)
		}, "The provided selector is empty")
	})

	t.Run("err_wrong_selector", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		p.SetContent(sampleHTML, nil)
		require.Panics(t, func() { p.TextContent("p", tb.toGojaValue(jsFrameBaseOpts{Timeout: "100"})) })
	})
}

func TestPageInputValue(t *testing.T) {
	p := newTestBrowser(t).NewPage(nil)

	p.SetContent(`
		<input value="hello1">
		<select><option value="hello2" selected></option></select>
		<textarea>hello3</textarea>
     	`, nil)

	got, want := p.InputValue("input", nil), "hello1"
	assert.Equal(t, got, want)

	got, want = p.InputValue("select", nil), "hello2"
	assert.Equal(t, got, want)

	got, want = p.InputValue("textarea", nil), "hello3"
	assert.Equal(t, got, want)
}

// test for: https://github.com/grafana/xk6-browser/issues/132
func TestPageInputSpecialCharacters(t *testing.T) {
	p := newTestBrowser(t).NewPage(nil)

	p.SetContent(`<input id="special">`, nil)
	el := p.Query("#special")

	wants := []string{
		"test@k6.io",
		"<hello WoRlD \\/>",
		"{(hello world!)}",
		"!#$%^&*()+_|~±",
		`¯\_(ツ)_/¯`,
	}
	for _, want := range wants {
		el.Fill("", nil)
		el.Type(want, nil)

		got := el.InputValue(nil)
		assert.Equal(t, want, got)
	}
}

func TestPageFill(t *testing.T) {
	p := newTestBrowser(t).NewPage(nil)
	p.SetContent(`
		<input id="text" type="text" value="something" />
		<input id="date" type="date" value="2012-03-12"/>
		<input id="number" type="number" value="42"/>
		<input id="unfillable" type="radio" />
	`, nil)

	happy := []struct{ name, selector, value string }{
		{name: "text", selector: "#text", value: "fill me up"},
		{name: "date", selector: "#date", value: "2012-03-13"},
		{name: "number", selector: "#number", value: "42"},
	}
	sad := []struct{ name, selector, value string }{
		{name: "date", selector: "#date", value: "invalid date"},
		{name: "number", selector: "#number", value: "forty two"},
		{name: "unfillable", selector: "#unfillable", value: "can't touch this"},
	}
	for _, tt := range happy {
		t.Run("happy/"+tt.name, func(t *testing.T) {
			p.Fill(tt.selector, tt.value, nil)
			require.Equal(t, tt.value, p.InputValue(tt.selector, nil))
		})
	}
	for _, tt := range sad {
		t.Run("sad/"+tt.name, func(t *testing.T) {
			require.Panics(t, func() { p.Fill(tt.selector, tt.value, nil) })
		})
	}
}

func TestPageIsChecked(t *testing.T) {
	p := newTestBrowser(t).NewPage(nil)

	p.SetContent(`<input type="checkbox" checked>`, nil)
	assert.True(t, p.IsChecked("input", nil), "expected checkbox to be checked")

	p.SetContent(`<input type="checkbox">`, nil)
	assert.False(t, p.IsChecked("input", nil), "expected checkbox to be unchecked")
}

func TestPageScreenshotFullpage(t *testing.T) {
	tb := newTestBrowser(t)
	p := tb.NewPage(nil)

	p.SetViewportSize(tb.toGojaValue(struct {
		Width  float64 `js:"width"`
		Height float64 `js:"height"`
	}{Width: 1280, Height: 800}))
	p.Evaluate(tb.toGojaValue(`
	() => {
		document.body.style.margin = '0';
		document.body.style.padding = '0';
		document.documentElement.style.margin = '0';
		document.documentElement.style.padding = '0';

		const div = document.createElement('div');
		div.style.width = '1280px';
		div.style.height = '800px';
		div.style.background = 'linear-gradient(red, blue)';

		document.body.appendChild(div);
	}
    	`))

	buf := p.Screenshot(tb.toGojaValue(struct {
		FullPage bool `js:"fullPage"`
	}{FullPage: true}))

	reader := bytes.NewReader(buf.Bytes())
	img, err := png.Decode(reader)
	assert.Nil(t, err)

	assert.Equal(t, 1280, img.Bounds().Max.X, "screenshot width is not 1280px as expected, but %dpx", img.Bounds().Max.X)
	assert.Equal(t, 800, img.Bounds().Max.Y, "screenshot height is not 800px as expected, but %dpx", img.Bounds().Max.Y)

	r, _, b, _ := img.At(0, 0).RGBA()
	assert.Greater(t, r, uint32(128))
	assert.Less(t, b, uint32(128))
	r, _, b, _ = img.At(0, 799).RGBA()
	assert.Less(t, r, uint32(128))
	assert.Greater(t, b, uint32(128))
}

func TestPageTitle(t *testing.T) {
	p := newTestBrowser(t).NewPage(nil)
	p.SetContent(`<html><head><title>Some title</title></head></html>`, nil)
	assert.Equal(t, "Some title", p.Title())
}

func TestPageSetExtraHTTPHeaders(t *testing.T) {
	b := newTestBrowser(t, withHTTPServer())

	p := b.NewPage(nil)

	headers := map[string]string{
		"Some-Header": "Some-Value",
	}
	p.SetExtraHTTPHeaders(headers)

	resp, err := p.Goto(b.URL("/get"), nil)
	require.NoError(t, err)
	require.NotNil(t, resp)

	var body struct{ Headers map[string][]string }
	err = json.Unmarshal(resp.Body().Bytes(), &body)
	require.NoError(t, err)

	h := body.Headers["Some-Header"]
	require.NotEmpty(t, h)
	assert.Equal(t, "Some-Value", h[0])
}

func TestPageWaitForFunction(t *testing.T) {
	t.Parallel()

	// script is here to test we're not getting an error from the
	// waitForFunction call itself and the tests that use it are
	// testing the polling functionality—not the response from
	// waitForFunction.
	script := `
		let resp = page.waitForFunction(%s, %s, %s)
		log('ok: '+resp);`

	t.Run("ok_func_raf_default", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		var log []string
		require.NoError(t, tb.runtime().Set("log", func(s string) { log = append(log, s) }))
		require.NoError(t, tb.runtime().Set("page", p))

		_, err := tb.runJavaScript(`fn = () => {
			if (typeof window._cnt == 'undefined') window._cnt = 0;
			if (window._cnt >= 50) return true;
			window._cnt++;
			return false;
		}`)
		require.NoError(t, err)

		_, err = tb.runJavaScript(script, "fn", "{}", "null")
		require.NoError(t, err)
		assert.Contains(t, log, "ok: null")
	})

	t.Run("ok_func_raf_default_arg", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		require.NoError(t, tb.runtime().Set("page", p))
		var log []string
		require.NoError(t, tb.runtime().Set("log", func(s string) { log = append(log, s) }))

		_, err := tb.runJavaScript(`fn = arg => {
			window._arg = arg;
			return true;
		}`)
		require.NoError(t, err)

		arg := "raf_arg"
		_, err = tb.runJavaScript(script, "fn", "{}", fmt.Sprintf("%q", arg))
		require.NoError(t, err)
		assert.Contains(t, log, "ok: null")

		argEvalJS := p.Evaluate(tb.toGojaValue("() => window._arg"))
		argEval, ok := argEvalJS.(goja.Value)
		require.True(t, ok)
		var gotArg string
		_ = tb.runtime().ExportTo(argEval, &gotArg)
		assert.Equal(t, arg, gotArg)
	})

	t.Run("ok_func_raf_default_args", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		require.NoError(t, tb.runtime().Set("page", p))
		var log []string
		require.NoError(t, tb.runtime().Set("log", func(s string) { log = append(log, s) }))

		_, err := tb.runJavaScript(`fn = (...args) => {
			window._args = args;
			return true;
		}`)
		require.NoError(t, err)

		args := []int{1, 2, 3}
		argsJS, err := json.Marshal(args)
		require.NoError(t, err)

		_, err = tb.runJavaScript(script, "fn", "{}", fmt.Sprintf("...%s", string(argsJS)))
		require.NoError(t, err)
		assert.Contains(t, log, "ok: null")

		argEvalJS := p.Evaluate(tb.toGojaValue("() => window._args"))
		argEval, ok := argEvalJS.(goja.Value)
		require.True(t, ok)
		var gotArgs []int
		_ = tb.runtime().ExportTo(argEval, &gotArgs)
		assert.Equal(t, args, gotArgs)
	})

	t.Run("err_expr_raf_timeout", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		rt := tb.vu.Runtime()
		var log []string
		require.NoError(t, rt.Set("log", func(s string) { log = append(log, s) }))
		require.NoError(t, rt.Set("page", p))

		_, err := tb.runJavaScript(script, "false", "{ polling: 'raf', timeout: 500, }", "null")
		require.ErrorContains(t, err, "timed out after 500ms")
	})

	t.Run("err_wrong_polling", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		rt := tb.vu.Runtime()
		require.NoError(t, rt.Set("page", p))

		_, err := tb.runJavaScript(script, "false", "{ polling: 'blah' }", "null")
		require.Error(t, err)
		assert.Contains(t, err.Error(),
			`parsing waitForFunction options: wrong polling option value:`,
			`"blah"; possible values: "raf", "mutation" or number`)
	})

	t.Run("ok_expr_poll_interval", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		require.NoError(t, tb.runtime().Set("page", p))
		var log []string
		require.NoError(t, tb.runtime().Set("log", func(s string) { log = append(log, s) }))

		p.Evaluate(tb.toGojaValue(`() => {
			setTimeout(() => {
				const el = document.createElement('h1');
				el.innerHTML = 'Hello';
				document.body.appendChild(el);
			}, 1000);
		}`))

		script := `
			let resp = page.waitForFunction(%s, %s, %s);
			if (resp) {
				log('ok: '+resp.innerHTML());
			} else {
				log('err: '+err);
			}`

		s := fmt.Sprintf(script, `"document.querySelector('h1')"`, "{ polling: 100, timeout: 2000, }", "null")
		_, err := tb.runJavaScript(s)
		require.NoError(t, err)
		assert.Contains(t, log, "ok: Hello")
	})

	t.Run("ok_func_poll_mutation", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		p := tb.NewPage(nil)
		require.NoError(t, tb.runtime().Set("page", p))
		var log []string
		require.NoError(t, tb.runtime().Set("log", func(s string) { log = append(log, s) }))

		_, err := tb.runJavaScript(`fn = () => document.querySelector('h1') !== null`)
		require.NoError(t, err)

		p.Evaluate(tb.toGojaValue(`() => {
			console.log('calling setTimeout...');
			setTimeout(() => {
				console.log('creating element...');
				const el = document.createElement('h1');
				el.innerHTML = 'Hello';
				document.body.appendChild(el);
			}, 1000);
		}`))

		s := fmt.Sprintf(script, "fn", "{ polling: 'mutation', timeout: 2000, }", "null")
		_, err = tb.runJavaScript(s)
		require.NoError(t, err)
		assert.Contains(t, log, "ok: null")
	})
}

func TestPageWaitForLoadState(t *testing.T) {
	t.Parallel()

	t.Run("err_wrong_event", func(t *testing.T) {
		t.Parallel()

		tb := newTestBrowser(t)
		assertExceptionContains(t, tb.runtime(), func() {
			p := tb.NewPage(nil)
			p.WaitForLoadState("none", nil)
		}, `invalid lifecycle event: "none"; must be one of: load, domcontentloaded, networkidle`)
	})
}

// See: The issue #187 for details.
func TestPageWaitForNavigationErrOnCtxDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := newTestBrowser(t, ctx).NewPage(nil)
	go cancel()
	<-ctx.Done()
	_, err := p.WaitForNavigation(nil)
	require.ErrorContains(t, err, "canceled")
}

func TestPagePress(t *testing.T) {
	tb := newTestBrowser(t)

	p := tb.NewPage(nil)

	p.SetContent(`<input id="text1">`, nil)

	p.Press("#text1", "Shift+KeyA", nil)
	p.Press("#text1", "KeyB", nil)
	p.Press("#text1", "Shift+KeyC", nil)

	require.Equal(t, "AbC", p.InputValue("#text1", nil))
}

func assertExceptionContains(t *testing.T, rt *goja.Runtime, fn func(), expErrMsg string) {
	t.Helper()

	cal, _ := goja.AssertFunction(rt.ToValue(fn))

	_, err := cal(goja.Undefined())
	require.ErrorContains(t, err, expErrMsg)
}
