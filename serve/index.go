package serve

import (
	"net/http"

	"github.com/hesusruiz/signers/redt"
	"github.com/labstack/echo/v4"
)

var indexHTML = `
<!DOCTYPE html>
<html>

<head>
    <title>Alastria RedT Validator activity</title>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <link rel="stylesheet" href="https://www.w3schools.com/w3css/4/w3.css">
    <link rel="stylesheet" href="https://www.w3schools.com/lib/w3-theme-teal.css">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/4.3.0/css/font-awesome.min.css">
</head>

<body>

    <header class="w3-container w3-theme">
        <h3>Alastria RedT Validators</h3>
    </header>

    <p id="mytable"></p>

    <script>
      var loc = window.location;
      var uri = 'ws:';
  
      if (loc.protocol === 'https:') {
        uri = 'wss:';
      }
      uri += '//' + loc.host;
      uri += loc.pathname + 'ws';
  
      ws = new WebSocket(uri)
  
      ws.onopen = function() {
        console.log('Connected')
      }
  
      ws.onmessage = function(evt) {
        var tt = document.getElementById('mytable')
        tt.innerHTML = evt.data
      }

      ws.onclose = function(evt) {
        location.reload()
      }

      ws.onerror = function(evt) {
        location.reload()
      }

    </script>

</body>
</html>
`

var tableHTML = `
<div class="w3-container">
    <div>
        <p>Block: {{.number}} ({{.elapsed}} sec) {{.timestamp}}</p>
        <p>GasLimit: {{.gasLimit}} GasUsed: {{.gasUsed}}</p>
    </div>
    <div class="w3-responsive w3-card-4">
        <table class="w3-table w3-striped w3-bordered">
            <thead>
                <tr class="w3-theme">
                    <th>Author</th>
                    <th>Signer</th>
                    <th>Name</th>
                </tr>
            </thead>
            <tbody>
                {{range .signers }}
                <tr>
                    <td>{{.authorCount}}</td>
                    <td>{{.signerCount}}</td>
                    <td>{{.operator}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    <div>
        <p>Next: {{.nextProposerOperator}}</p>
    </div>
</div>
`

func renderIndex(c echo.Context, rt *redt.RedTNode) error {
	return c.HTML(http.StatusOK, indexHTML)
}

// func renderIndex(c echo.Context, rt *redt.RedTNode) error {

// 	data, err := os.ReadFile("serve/index.html")
// 	if err != nil {
// 		c.Logger().Error(err)
// 		return err
// 	}
// 	return c.HTMLBlob(http.StatusOK, data)
// }
