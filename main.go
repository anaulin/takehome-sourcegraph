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

func RepositoriesHandler(w http.ResponseWriter, r *http.Request) {
	count := 1
	unique := false
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

	if count > 6 && unique {
		log.Fatal("Requested unique and a count larger than total number of unique repos. That won't work.")
	}

	ch := make(chan *repository)
	for i := 0; i < count; i++ {
		go getRepository(ch)
	}

	var repos []repository
	repoIndex := make(map[int]interface{})
	for {
		r := <-ch
		_, alreadyGotIt := repoIndex[r.ID]
		if unique && alreadyGotIt {
			go getRepository(ch)
		} else {
			repoIndex[r.ID] = true
			repos = append(repos, *r)
		}
		if len(repos) == count {
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")
	reposResponse := map[string]interface{}{"repositories": repos}
	if err := json.NewEncoder(w).Encode(reposResponse); err != nil {
		panic(err)
	}
}

// TODO(anaulin): Turn this into a parameter provided to the proxy server at startup.
const codehost string = "http://localhost:7080"
const repositoryURL string = codehost + "/repository?failRatio=0.7"

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
	http.HandleFunc("/repositories", RepositoriesHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}
