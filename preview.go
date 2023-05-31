package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"

	"github.com/pkg/browser"
)

func preview(fig FigureData) error {
	figBytes, err := json.Marshal(fig)
	if err != nil {
		return fmt.Errorf("marshal fig: %w", err)
	}

	tmpl, err := template.New("plotly").Parse(baseHtml)
	if err != nil {
		return fmt.Errorf("parse plotly html: %w", err)
	}

	buf := &bytes.Buffer{}
	if err = tmpl.Execute(buf, string(figBytes)); err != nil {
		return fmt.Errorf("html template: %w", err)
	}

	return browser.OpenReader(buf)
}

var baseHtml = `
<html>
   <head>
      <script src="https://cdn.plot.ly/plotly-1.58.4.min.js"></script>
      <style id="plotly.js-style-global"></style>
      <style id="plotly.js-style-modebar-2e7662"></style>

      <link href="https://cdnjs.cloudflare.com/ajax/libs/jsoneditor/9.10.0/jsoneditor.css" rel="stylesheet" type="text/css">
      <script src="https://cdnjs.cloudflare.com/ajax/libs/jsoneditor/9.10.0/jsoneditor.min.js"></script>
      <script src="https://cdnjs.cloudflare.com/ajax/libs/js-yaml/4.1.0/js-yaml.min.js"></script>
   </head>
   <body>
      <div id="plot" class="js-plotly-plot"></div>
      <div>
         <p>
            <button onclick="updatePlot();">Update</button>
            <button id="copyYamlBtn" onclick="copyYaml();">Copy layout yaml</button>
         </p>
         <div id="jsoneditor" style="width: 100%; height: 800px;"></div>
      </div>
      
      <script>
        const data = JSON.parse('{{ . }}')
         
        Plotly.newPlot('plot', data);

        const container = document.getElementById("jsoneditor");
        const editor = new JSONEditor(container, { mode: 'code' }, data);

        function updatePlot() {
            Plotly.newPlot('plot', editor.get());
        }

        function copyYaml() {
            // get json and delete data field
            var json = editor.get();
            delete json.data;

            // copy to clipboard
            navigator.clipboard.writeText(jsyaml.dump(json));

            // provide visual feedback
            const container = document.getElementById("copyYamlBtn");
            container.innerText = "Copied!";
            setTimeout(function() {
                container.innerText = "Copy layout yaml";
            }, 1000);
        }
      </script>
   </body>
</html>
	`
