package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"slices"

	"github.com/buke/quickjs-go"
	esbuild "github.com/evanw/esbuild/pkg/api"
)

var production = true
var ssrPath = "./web/ssr.jsx"
var hydratePath = "./web/hydrate.jsx"

func main() {
	rt := quickjs.NewRuntime()
	defer rt.Close()
	ctx := rt.NewContext()
	defer ctx.Close()

	processPolyfill := `var process = { env: { NODE_ENV: "production" } };`
	consolePolyfill := `var console = { log: function(){} };`
	// https://gist.github.com/Yaffle/5458286
	textEncPolyfill := `function TextEncoder(){} TextEncoder.prototype.encode=function(string){var octets=[],length=string.length,i=0;while(i<length){var codePoint=string.codePointAt(i),c=0,bits=0;codePoint<=0x7F?(c=0,bits=0x00):codePoint<=0x7FF?(c=6,bits=0xC0):codePoint<=0xFFFF?(c=12,bits=0xE0):codePoint<=0x1FFFFF&&(c=18,bits=0xF0),octets.push(bits|(codePoint>>c)),c-=6;while(c>=0){octets.push(0x80|((codePoint>>c)&0x3F)),c-=6}i+=codePoint>=0x10000?2:1}return octets};function TextDecoder(){} TextDecoder.prototype.decode=function(octets){var string="",i=0;while(i<octets.length){var octet=octets[i],bytesNeeded=0,codePoint=0;octet<=0x7F?(bytesNeeded=0,codePoint=octet&0xFF):octet<=0xDF?(bytesNeeded=1,codePoint=octet&0x1F):octet<=0xEF?(bytesNeeded=2,codePoint=octet&0x0F):octet<=0xF4&&(bytesNeeded=3,codePoint=octet&0x07),octets.length-i-bytesNeeded>0?function(){for(var k=0;k<bytesNeeded;){octet=octets[i+k+1],codePoint=(codePoint<<6)|(octet&0x3F),k+=1}}():codePoint=0xFFFD,bytesNeeded=octets.length-i,string+=String.fromCodePoint(codePoint),i+=bytesNeeded+1}return string};`

	ssrJScripts := []any{
		bundleJS(ssrPath, production),                     // SSR React
		textEncPolyfill, consolePolyfill, processPolyfill, // Polyfills
	}

	slices.Reverse(ssrJScripts)                // Polyfills first, reverse items order
	ssrJScript := fmt.Sprintln(ssrJScripts...) // Concatenate all scripts
	fmt.Println("* Bundled SSR Javascript length:", len(ssrJScript))

	// Pre-evaluate the bundled script for SSR
	_, err := ctx.Eval(ssrJScript)
	if err != nil {
		log.Fatalf("failed to evaluate SSR Javascript: %v", err)
	}

	htmlTmpl := `
	<!DOCTYPE html>
	<html lang="en">
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>PAGE</title>
		<script>window.APP_PROPS = {{ index . "appProps" }};</script>
	</head>
	<body>
		<div id="app">{{ index . "appHTML" }}</div>
		<script type="module">
		{{ index . "javascript" }}
		</script>
	</body>
	</html>`

	// TODO: Pages structure & routing
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Bundle the react hydrate script for interactive components
		hydrateJScript := bundleJS(hydratePath, production)

		appProps, _ := json.Marshal(map[string]any{
			"message": "Hello, World! from Golang.",
		})

		// Render the React component
		html, err := ctx.Eval(fmt.Sprintf("render(%s)", string(appProps)))
		defer html.Free()
		if err != nil {
			log.Fatalf("server failed to render React: %v", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// Extract the HTML string from the QuickJS value
		appHTML := html.String()
		fmt.Printf("* React rendered to HTML string:\n%s\n", appHTML)

		w.Header().Set("Content-Type", "text/html")
		tmpl, err := template.New("page").Parse(htmlTmpl)
		if err != nil {
			log.Fatalf("failed parsing page template: %s", err.Error())
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		// render the HTML webpage
		err = tmpl.Execute(w, map[string]any{
			"javascript": template.JS(hydrateJScript),
			"appHTML":    template.HTML(appHTML),
			"appProps":   template.JS(appProps),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	fmt.Println("Serving at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func bundleJS(filepath string, production bool) string {
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints: []string{filepath},
		Bundle:      true,
		Write:       false,
		Format:      esbuild.FormatIIFE,
		Platform:    esbuild.PlatformBrowser,
		Target:      esbuild.ES2015,
		Loader: map[string]esbuild.Loader{
			".jsx": esbuild.LoaderJSX,
		},
		// Minify
		MinifyWhitespace:  production,
		MinifyIdentifiers: production,
		MinifySyntax:      production,
	})

	return string(result.OutputFiles[0].Contents)
}
