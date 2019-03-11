package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

type repository struct {
	ID        int       `json:"id,omitempty"`
	Name      string    `json:"name,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

type repoResponse struct {
	Repo repository `json:"repository"`
}

// TODO(anaulin): Turn this into a parameter provided to the proxy server at startup.
const codehost string = "http://localhost:7080"
const repositoryURL string = codehost + "/repository"

// A cache of repos that we keep around between requests. Yay global variables.
var repoCache map[int]repository

func RepositoriesHandler(w http.ResponseWriter, r *http.Request) {
	count := 1
	unique := false
	timeoutMs := 1000
	query := r.URL.Query()
	if count_params, ok := query["count"]; ok && len(count_params) > 0 {
		new_count, err := strconv.Atoi(count_params[0])
		if err == nil {
			count = new_count
		}
	}
	if unique_params, ok := query["unique"]; ok && len(unique_params) > 0 {
		if unique_params[0] == "true" {
			unique = true
		}
	}
	if timeoutParams, ok := query["timeout"]; ok && len(timeoutParams) > 0 {
		newTimeoutMs, err := strconv.Atoi(timeoutParams[0])
		if err == nil {
			timeoutMs = newTimeoutMs
		}
	}

	if count > 6 && unique {
		log.Fatal("Requested unique and a count larger than total number of unique repos. That won't work.")
	}

	ch := make(chan *repository)
	for i := 0; i < count; i++ {
		go getRepository(ch)
	}

	var repos []repository
	repoIndex := make(map[int]interface{})

requestLoop:
	for {
		select {
		case <-time.After(time.Duration(timeoutMs) * time.Millisecond):
			log.Print("timeout")
			for _, repo := range repoCache {
				if len(repos) < count {
					handleRepository(unique, &repo, &repos, &repoIndex)
				}
			}
			break requestLoop
		case r := <-ch:
			if !handleRepository(unique, r, &repos, &repoIndex) {
				go getRepository(ch)
			}
			if len(repos) == count {
				break requestLoop
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	reposResponse := map[string]interface{}{"repositories": repos}
	if err := json.NewEncoder(w).Encode(reposResponse); err != nil {
		panic(err)
	}
}

// Returns 'true' if a new repository was added to the repos slice.
func handleRepository(unique bool, r *repository, repos *[]repository, repoIndex *(map[int]interface{})) bool {
	repoCache[r.ID] = *r
	_, alreadyGotIt := (*repoIndex)[r.ID]
	if unique && alreadyGotIt {
		return false
	}
	(*repoIndex)[r.ID] = true
	*repos = append(*repos, *r)
	return true
}

func getRepository(ch chan<- *repository) {
	resp, err := http.Get(repositoryURL)
	for err != nil {
		resp, err = http.Get(repositoryURL)
	}
	body, _ := ioutil.ReadAll(resp.Body)

	var repoRes repoResponse
	json.Unmarshal(body, &repoRes)

	ch <- &repoRes.Repo
}

func main() {
	repoCache = make(map[int]repository)
	http.HandleFunc("/repositories", RepositoriesHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
