package serve

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"text/template"

	"github.com/hesusruiz/signers/client"
	"github.com/hesusruiz/signers/redt"
	"github.com/hesusruiz/signers/types"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/labstack/gommon/log"

	ethertypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/gorilla/websocket"
)

type Template struct {
	templates *template.Template
}

func (t *Template) Srender(name string, data interface{}, c echo.Context) (bytes.Buffer, error) {
	var rendered bytes.Buffer
	err := t.templates.ExecuteTemplate(&rendered, name, data)
	if err != nil {
		c.Logger().Error(err)
	}
	return rendered, err
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	err := t.templates.ExecuteTemplate(w, name, data)
	if err != nil {
		c.Logger().Error(err)
	}
	return err
}

type Server struct {
	rt *redt.RedTNode
}

func ServeSigners(url string, ip string, port int64) {
	var err error

	serverIP := fmt.Sprintf("%v:%v", ip, port)

	// Preprocess the template for better performance
	t := &Template{
		templates: template.Must(template.New("table.html").Parse(tableHTML)),
	}

	// Create the server struct
	server := &Server{}

	// Connect to the RedT node
	rt, err := redt.NewRedTNode(url)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

	server.rt = rt

	// Create an instance of web server
	e := echo.New()

	// Configure the logger
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "method=${method}, uri=${uri}, status=${status}\n",
	}))
	e.Logger.SetLevel(log.DEBUG)

	// Recover from panics so the server can continue
	e.Use(middleware.Recover())

	// Middleware for rendering with templates
	e.Renderer = t

	// Calling to this route upgrades http to a WebSocket connection
	e.GET("/ws", func(c echo.Context) error {
		return server.serveViaWS(c, url)
	})

	// The root serves an HTML with Javascript to start WebSocket from the browser
	e.GET("/", func(c echo.Context) error {
		return renderIndex(c, rt)
	})

	// Start the server listening on the specified ip:port
	e.Logger.Fatal(e.Start(serverIP))
}

var (
	upgrader = websocket.Upgrader{}
)

func (s *Server) serveViaWS(c echo.Context, url string) error {

	var rendered bytes.Buffer
	var data map[string]any

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	t := template.Must(template.New("table.html").Parse(tableHTML))

	qc, err := client.NewQuorumClient(url)
	if err != nil {
		log.Error(err)
		return err
	}
	defer qc.Stop()

	inputCh := make(chan types.RawHeader)

	err = qc.SubscribeChainHead(inputCh)
	if err != nil {
		log.Error(err)
		return err
	}

	latestTimestamp := uint64(0)

	isFirst := true

	for {

		// Block receiving headers from the channel
		rawheader := <-inputCh

		var currentHeader *ethertypes.Header

		// Get the full header, because the raw one does not have the info we need
		currentHeader, err = s.rt.HeaderByNumber(int64(rawheader.Number))
		if err != nil {
			// Log the error and retry with next block
			log.Error(err)
			return err
		}

		if isFirst {
			// Do not display, we just get its timestamp to start statistics
			latestTimestamp = currentHeader.Time
			isFirst = false

			// Wait for the next block
			continue
		}

		// Get the signer data and accumulated statistics
		data, latestTimestamp = s.rt.SignersForHeader(currentHeader, latestTimestamp)

		// Format the data into an HTML table
		rendered.Reset()
		err = t.ExecuteTemplate(&rendered, "table.html", data)
		if err != nil {
			log.Error(err)
			return err
		}

		// Send the HTML table to the client via the WebSocket connection
		err = ws.WriteMessage(websocket.TextMessage, rendered.Bytes())
		if err != nil {
			log.Error(err)
			return err
		}

	}

}
