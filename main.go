// Command codehost simulates a simple code host API.
// It can be configured to have high latency and
// to misbehave in different ways through query parameters.
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

var colors = []string{
	"red",
	"orange",
	"yellow",
	"green",
	"indigo",
	"violet",
	"", // intentionally empty
}

type repository struct {
	ID        int       `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

var repositories []repository

var port string

func main() {
	// Initialize more repositories than unique names.
	for i := 0; i < 10; i++ {
		repositories = append(repositories, repository{
			ID:   i + 1,
			Name: colors[i%len(colors)],
		})
	}

	port = os.Getenv("PORT")
	if port == "" {
		port = "7080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleInstructions)
	mux.HandleFunc("/repository", handleGetRepository)

	log.Println("listening on http://localhost:" + port)
	log.Fatalln(http.ListenAndServe(":"+port, mux))
}

var instructionsTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
	<title>Instructions</title>
</head>
<body>
	<h1>Instructions</h1>
	<p>
		This service pretends to be a black box code host that exposes a simple API.
		Your task will be to build a proxy service that calls this code host and exposes a more advanced API.
	</p>
	<p>
		You are allowed to lookup documentation and information on the internet, but all code should be written by you during the time you have been given to complete this assignment.
	</p>
	<p>
		You might not have time to complete all tasks.
		Do not move on to another part of the problem until you have a complete solution to the previous part.
		You may even want to checkpoint your work with a git commit after each part.
	</p>
	<h1>Part 1: Aggregation</h1>
	<p><a href="{{.Addr}}">{{.Addr}}</a> returns a single random repository.</p>
	<p>Your task is to create a new service with an endpoint (e.g. <code>/repositories</code>) that calls <a href="{{.Addr}}">{{.Addr}}</a> and returns a JSON array of repositories.</p>
	<p>You endpoint should accept two query parameters:</p>
	<dl>
		<dt><code>count</code></dt>
		<dd>
			Return this number of random repositories (default to 1).
			Fetching multiple repositories from the code host should be done concurrently.
			You can use the <code>latency</code> query parameter to customize the latency of the code host (e.g. <a href="{{.Addr}}?latency=1s">{{.Addr}}?latency=1s</a>). The default latency is 500ms.
		</dd>
		<dt><code>unique</code></dt>
		<dd>Only return unique repositories if the value is <code>true</code>, else duplicates are allowed.</dd>
	</ul>

	<h1>Part 2: Error handling</h1>
	<p>Update your proxy to handle transient failures on the code host by retrying until the requirements of the request can be satisfied.</p>
	<p>You can simulate a high failure rate on the code host by setting the <code>failRatio</code> query parameter (e.g. <a href="{{.Addr}}?failRatio=0.7">{{.Addr}}?failRatio=0.7</a>).</p>

	<h1>Part 3: Timeout and caching</h1>
	<p>If the code host has a very high failure rate (or is completely down), then retrying indefinitely is not desirable.</p>
	<p>
		Add a <code>timeout</code> query parameter.
		When the timeout for a request is reached, the proxy should stop retrying and immediately attempt to satisfy the requirements of the request by backfilling the response using cached repositories from previous requests.
		If there isn't sufficient data in the cache, the proxy should return the data that it does have.
	</p>
</body>
</html>
`))

func handleInstructions(w http.ResponseWriter, r *http.Request) {
	if err := instructionsTemplate.Execute(w, map[string]interface{}{
		"Addr": fmt.Sprintf("http://localhost:%s/repository", port),
	}); err != nil {
		log.Println(err)
	}
}

func handleGetRepository(w http.ResponseWriter, r *http.Request) {
	id, err := idFormValue(r)
	if err != nil {
		writeError(w, err)
		return
	}
	repo := repositories[id-1]
	repo.FetchedAt = time.Now() // we are modifying a copy of the repo
	if err := misbehave(w, r, &repo); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"repository": repo,
	})
}

func idFormValue(r *http.Request) (int, error) {
	id := r.FormValue("id")
	if id == "" {
		return rand.Intn(len(repositories)) + 1, nil
	}
	i, err := strconv.Atoi(id)
	if err != nil {
		return 0, &httpError{
			status:  http.StatusBadRequest,
			message: err.Error(),
		}
	}
	return i, nil
}

func misbehave(w http.ResponseWriter, r *http.Request, repo *repository) error {
	latency := r.FormValue("latency")
	if latency == "" {
		latency = "500ms"
	}
	d, err := time.ParseDuration(latency)
	if err != nil {
		return err
	}
	time.Sleep(d)

	failRatio := r.FormValue("failRatio")
	if failRatio != "" {
		f, err := strconv.ParseFloat(failRatio, 64)
		if err != nil {
			return err
		}
		if rand.Float64() < f {
			switch rand.Intn(3) {
			case 0:
				r.Form.Set("panicRatio", "1")
			case 1:
				r.Form.Set("errorRatio", "1")
			case 2:
				r.Form.Set("garbageRatio", "1")
			default:
				panic("bug")
			}
		}
	}

	panicRatio := r.FormValue("panicRatio")
	if panicRatio != "" {
		f, err := strconv.ParseFloat(panicRatio, 64)
		if err != nil {
			return err
		}
		if rand.Float64() < f {
			panic("random panic!")
		}
	}

	errorRatio := r.FormValue("errorRatio")
	if errorRatio != "" {
		f, err := strconv.ParseFloat(errorRatio, 64)
		if err != nil {
			return err
		}
		if rand.Float64() < f {
			return &httpError{
				status:  http.StatusInternalServerError,
				message: "random error!",
			}
		}
	}

	garbageRatio := r.FormValue("garbageRatio")
	if garbageRatio != "" {
		f, err := strconv.ParseFloat(garbageRatio, 64)
		if err != nil {
			return err
		}
		if rand.Float64() < f {
			w.Write([]byte("random garbage!"))
		}
	}

	return nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		panic(err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	switch x := err.(type) {
	case *httpError:
		w.WriteHeader(x.status)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}
	writeJSON(w, map[string]string{
		"error": err.Error(),
	})
}

type httpError struct {
	status  int
	message string
}

func (err *httpError) Error() string {
	return err.message
}
