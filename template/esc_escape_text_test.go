// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Copied from html/template/escape_test.go of go1.27.3, with the changes marked below.

package template

// Changed: only testing and htt are imported; the loop uses no other package.
import (
	"testing"

	"github.com/typelate/htt"
)

func TestEscapeText(t *testing.T) {
	tests := []struct {
		input  string
		output context
	}{
		{
			``,
			context{},
		},
		{
			`Hello, World!`,
			context{},
		},
		{
			// An orphaned "<" is OK.
			`I <3 Ponies!`,
			context{},
		},
		{
			`<a`,
			context{state: stateTag},
		},
		{
			`<a `,
			context{state: stateTag},
		},
		{
			`<a>`,
			context{state: stateText},
		},
		{
			`<a href`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a on`,
			context{state: stateAttrName, attr: attrScript},
		},
		{
			`<a href `,
			context{state: stateAfterName, attr: attrURL},
		},
		{
			`<a style  =  `,
			context{state: stateBeforeValue, attr: attrStyle},
		},
		{
			`<a href=`,
			context{state: stateBeforeValue, attr: attrURL},
		},
		{
			`<a href=x`,
			context{state: stateURL, delim: delimSpaceOrTagEnd, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href=x `,
			context{state: stateTag},
		},
		{
			`<a href=>`,
			context{state: stateText},
		},
		{
			`<a href=x>`,
			context{state: stateText},
		},
		{
			`<a href ='`,
			context{state: stateURL, delim: delimSingleQuote, attr: attrURL},
		},
		{
			`<a href=''`,
			context{state: stateTag},
		},
		{
			`<a href= "`,
			context{state: stateURL, delim: delimDoubleQuote, attr: attrURL},
		},
		{
			`<a href=""`,
			context{state: stateTag},
		},
		{
			`<a title="`,
			context{state: stateAttr, delim: delimDoubleQuote},
		},
		{
			`<a HREF='http:`,
			context{state: stateURL, delim: delimSingleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a Href='/`,
			context{state: stateURL, delim: delimSingleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href='"`,
			context{state: stateURL, delim: delimSingleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href="'`,
			context{state: stateURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href='&apos;`,
			context{state: stateURL, delim: delimSingleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href="&quot;`,
			context{state: stateURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href="&#34;`,
			context{state: stateURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<a href=&quot;`,
			context{state: stateURL, delim: delimSpaceOrTagEnd, urlPart: urlPartPreQuery, attr: attrURL},
		},
		{
			`<img alt="1">`,
			context{state: stateText},
		},
		{
			`<img alt="1>"`,
			context{state: stateTag},
		},
		{
			`<img alt="1>">`,
			context{state: stateText},
		},
		{
			`<input checked type="checkbox"`,
			context{state: stateTag},
		},
		{
			`<a onclick="`,
			context{state: stateJS, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="//foo`,
			context{state: stateJSLineCmt, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			"<a onclick='//\n",
			context{state: stateJS, delim: delimSingleQuote, attr: attrScript},
		},
		{
			"<a onclick='//\r\n",
			context{state: stateJS, delim: delimSingleQuote, attr: attrScript},
		},
		{
			"<a onclick='//\u2028",
			context{state: stateJS, delim: delimSingleQuote, attr: attrScript},
		},
		{
			`<a onclick="/*`,
			context{state: stateJSBlockCmt, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/*/`,
			context{state: stateJSBlockCmt, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/**/`,
			context{state: stateJS, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onkeypress="&quot;`,
			context{state: stateJSDqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick='&quot;foo&quot;`,
			context{state: stateJS, delim: delimSingleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<a onclick=&#39;foo&#39;`,
			context{state: stateJS, delim: delimSpaceOrTagEnd, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<a onclick=&#39;foo`,
			context{state: stateJSSqStr, delim: delimSpaceOrTagEnd, attr: attrScript},
		},
		{
			`<a onclick="&quot;foo'`,
			context{state: stateJSDqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="'foo&quot;`,
			context{state: stateJSSqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			"<a onclick=\"`foo",
			context{state: stateJSTmplLit, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<A ONCLICK="'`,
			context{state: stateJSSqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/`,
			context{state: stateJSRegexp, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="'foo'`,
			context{state: stateJS, delim: delimDoubleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<a onclick="'foo\'`,
			context{state: stateJSSqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="'foo\'`,
			context{state: stateJSSqStr, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/foo/`,
			context{state: stateJS, delim: delimDoubleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<script>/foo/ /=`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<a onclick="1 /foo`,
			context{state: stateJS, delim: delimDoubleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<a onclick="1 /*c*/ /foo`,
			context{state: stateJS, delim: delimDoubleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<a onclick="/foo[/]`,
			context{state: stateJSRegexp, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/foo\/`,
			context{state: stateJSRegexp, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<a onclick="/foo/`,
			context{state: stateJS, delim: delimDoubleQuote, jsCtx: jsCtxDivOp, attr: attrScript},
		},
		{
			`<input checked style="`,
			context{state: stateCSS, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="//`,
			context{state: stateCSSLineCmt, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="//</script>`,
			context{state: stateCSSLineCmt, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			"<a style='//\n",
			context{state: stateCSS, delim: delimSingleQuote, attr: attrStyle},
		},
		{
			"<a style='//\r",
			context{state: stateCSS, delim: delimSingleQuote, attr: attrStyle},
		},
		{
			`<a style="/*`,
			context{state: stateCSSBlockCmt, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="/*/`,
			context{state: stateCSSBlockCmt, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="/**/`,
			context{state: stateCSS, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: '`,
			context{state: stateCSSSqStr, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: &quot;`,
			context{state: stateCSSDqStr, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: '/foo?img=`,
			context{state: stateCSSSqStr, delim: delimDoubleQuote, urlPart: urlPartQueryOrFrag, attr: attrStyle},
		},
		{
			`<a style="background: '/`,
			context{state: stateCSSSqStr, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url(&#x22;/`,
			context{state: stateCSSDqURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url('/`,
			context{state: stateCSSSqURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url('/)`,
			context{state: stateCSSSqURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url('/ `,
			context{state: stateCSSSqURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url(/`,
			context{state: stateCSSURL, delim: delimDoubleQuote, urlPart: urlPartPreQuery, attr: attrStyle},
		},
		{
			`<a style="background: url( `,
			context{state: stateCSSURL, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: url( /image?name=`,
			context{state: stateCSSURL, delim: delimDoubleQuote, urlPart: urlPartQueryOrFrag, attr: attrStyle},
		},
		{
			`<a style="background: url(x)`,
			context{state: stateCSS, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: url('x'`,
			context{state: stateCSS, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<a style="background: url( x `,
			context{state: stateCSS, delim: delimDoubleQuote, attr: attrStyle},
		},
		{
			`<!-- foo`,
			context{state: stateHTMLCmt},
		},
		{
			`<!-->`,
			context{state: stateHTMLCmt},
		},
		{
			`<!--->`,
			context{state: stateHTMLCmt},
		},
		{
			`<!-- foo -->`,
			context{state: stateText},
		},
		{
			`<script`,
			context{state: stateTag, element: elementScript},
		},
		{
			`<script `,
			context{state: stateTag, element: elementScript},
		},
		{
			`<script src="foo.js" `,
			context{state: stateTag, element: elementScript},
		},
		{
			`<script src='foo.js' `,
			context{state: stateTag, element: elementScript},
		},
		{
			`<script type=text/javascript `,
			context{state: stateTag, element: elementScript},
		},
		{
			`<script>`,
			context{state: stateJS, jsCtx: jsCtxRegexp, element: elementScript},
		},
		{
			`<script>foo`,
			context{state: stateJS, jsCtx: jsCtxDivOp, element: elementScript},
		},
		{
			`<script>foo</script>`,
			context{state: stateText},
		},
		{
			`<script>foo</script><!--`,
			context{state: stateHTMLCmt},
		},
		{
			`<script>document.write("<p>foo</p>");`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<script>document.write("<p>foo<\/script>");`,
			context{state: stateJS, element: elementScript},
		},
		{
			// <script and </script tags are escaped, so </script> should not
			// cause us to exit the JS state.
			`<script>document.write("<script>alert(1)</script>");`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<script>document.write("<script>`,
			context{state: stateJSDqStr, element: elementScript},
		},
		{
			`<script>document.write("<script>alert(1)</script>`,
			context{state: stateJSDqStr, element: elementScript},
		},
		{
			`<script>document.write("<script>alert(1)<!--`,
			context{state: stateJSDqStr, element: elementScript},
		},
		{
			`<script>document.write("<script>alert(1)</Script>");`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<script>document.write("<!--");`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<script>let a = /</script`,
			context{state: stateJSRegexp, element: elementScript},
		},
		{
			`<script>let a = /</script/`,
			context{state: stateJS, element: elementScript, jsCtx: jsCtxDivOp},
		},
		{
			`<script type="text/template">`,
			context{state: stateText},
		},
		// covering issue 19968
		{
			`<script type="TEXT/JAVASCRIPT">`,
			context{state: stateJS, element: elementScript},
		},
		// covering issue 19965
		{
			`<script TYPE="text/template">`,
			context{state: stateText},
		},
		{
			`<script type="notjs">`,
			context{state: stateText},
		},
		{
			`<Script>`,
			context{state: stateJS, element: elementScript},
		},
		{
			`<SCRIPT>foo`,
			context{state: stateJS, jsCtx: jsCtxDivOp, element: elementScript},
		},
		{
			`<textarea>value`,
			context{state: stateRCDATA, element: elementTextarea},
		},
		{
			`<textarea>value</TEXTAREA>`,
			context{state: stateText},
		},
		{
			`<textarea name=html><b`,
			context{state: stateRCDATA, element: elementTextarea},
		},
		{
			`<title>value`,
			context{state: stateRCDATA, element: elementTitle},
		},
		{
			`<style>value`,
			context{state: stateCSS, element: elementStyle},
		},
		{
			`<a xlink:href`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a xmlns`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a xmlns:foo`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a xmlnsxyz`,
			context{state: stateAttrName},
		},
		{
			`<a data-url`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a data-iconUri`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a data-urlItem`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a g:`,
			context{state: stateAttrName},
		},
		{
			`<a g:url`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a g:iconUri`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a g:urlItem`,
			context{state: stateAttrName, attr: attrURL},
		},
		{
			`<a g:value`,
			context{state: stateAttrName},
		},
		{
			`<a svg:style='`,
			context{state: stateCSS, delim: delimSingleQuote, attr: attrStyle},
		},
		{
			`<svg:font-face`,
			context{state: stateTag},
		},
		{
			`<svg:a svg:onclick="`,
			context{state: stateJS, delim: delimDoubleQuote, attr: attrScript},
		},
		{
			`<svg:a svg:onclick="x()">`,
			context{},
		},
		{
			"<script>var a = `",
			context{state: stateJSTmplLit, element: elementScript},
		},
		{
			"<script>var a = `${",
			context{state: stateJS, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${}",
			context{state: stateJSTmplLit, element: elementScript},
		},
		{
			"<script>var a = `${`",
			context{state: stateJSTmplLit, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${var a = \"",
			context{state: stateJSDqStr, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${var a = \"`",
			context{state: stateJSDqStr, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${var a = \"}",
			context{state: stateJSDqStr, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${``",
			context{state: stateJS, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${`}",
			context{state: stateJSTmplLit, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>`${ {} } asd`</script><script>`${ {} }",
			context{state: stateJSTmplLit, element: elementScript},
		},
		{
			"<script>var foo = `${ (_ => { return \"x\" })() + \"${",
			context{state: stateJSDqStr, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var a = `${ {</script><script>var b = `${ x }",
			context{state: stateJSTmplLit, element: elementScript, jsCtx: jsCtxDivOp},
		},
		{
			"<script>var foo = `x` + \"${",
			context{state: stateJSDqStr, element: elementScript},
		},
		{
			"<script>function f() { var a = `${}`; }",
			context{state: stateJS, element: elementScript},
		},
		{
			"<script>{`${}`}",
			context{state: stateJS, element: elementScript},
		},
		{
			"<script>`${ function f() { return `${1}` }() }`",
			context{state: stateJS, element: elementScript, jsCtx: jsCtxDivOp},
		},
		{
			"<script>function f() {`${ function f() { `${1}` } }`}",
			context{state: stateJS, element: elementScript, jsCtx: jsCtxRegexp},
		},
		{
			"<script>`${ { `` }",
			context{state: stateJS, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>`${ { }`",
			context{state: stateJSTmplLit, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>var foo = `${ foo({ a: { c: `${",
			context{state: stateJS, element: elementScript, jsBraceDepth: []int{2, 0}},
		},
		{
			"<script>var foo = `${ foo({ a: { c: `${ {{.}} }` }, b: ",
			context{state: stateJS, element: elementScript, jsBraceDepth: []int{1}},
		},
		{
			"<script>`${ `}",
			context{state: stateJSTmplLit, element: elementScript, jsBraceDepth: []int{0}},
		},
		{
			"<script>`${1}${",
			context{state: stateJS, element: elementScript, jsCtx: jsCtxRegexp, jsBraceDepth: []int{0}},
		},
		{
			"<script>`${`${1}${",
			context{state: stateJS, element: elementScript, jsCtx: jsCtxRegexp, jsBraceDepth: []int{0, 0}},
		},
	}

	for _, test := range tests {
		// Changed: the port takes the text as an argument and records its rewrite
		// in the plan, so the node is built over the same bytes.
		b := []byte(test.input)
		e := makeEscaper(nil)
		c := e.escapeText(context{}, &htt.TextNode{Text: b}, b)
		if !test.output.eq(c) {
			t.Errorf("input %q: want context\n\t%v\ngot\n\t%v", test.input, test.output, c)
			continue
		}
		// Changed: the port does not modify the bytes it is given, so this asserts they are unchanged.
		if test.input != string(b) {
			t.Errorf("input %q: text passed in was modified: want %q got %q", test.input, test.input, b)
			continue
		}
	}
}
