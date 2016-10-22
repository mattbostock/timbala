package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

var baseURL string

type apiQueryData struct {
	ResultType model.ValueType  `json:"resultType"`
	Result     model.SamplePair `json:"result"`
}
type apiResponse struct {
	Status    string       `json:"status"`
	Data      apiQueryData `json:"data,omitempty"`
	ErrorType string       `json:"errorType,omitempty"`
	Error     string       `json:"error,omitempty"`
}

func TestSimpleArithmeticQuery(t *testing.T) {
	query := "1+1"
	expected := "2"

	queryAPI(t, query, expected)
}

func TestMain(m *testing.M) {
	flag.Parse()

	// Use localhost to avoid firewall warnings when running tests under OS X.
	config.listenAddr = "localhost:9080"

	baseURL = fmt.Sprintf("http://%s", config.listenAddr)
	go main()

	err := waitForServer(baseURL)
	if err != nil {
		log.Fatal("Test setup failed: ", err)
	}

	os.Exit(m.Run())
}

func queryAPI(t *testing.T, query, expected string) {
	resp, err := http.Get(queryURL(query))
	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("Error reading response body: ", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("Got response code %d, expected %s", resp.StatusCode, 200)
	}

	if h := resp.Header.Get("Content-Type"); h != "application/json" {
		t.Fatalf("Expected Content-Type %q, got %q", "application/json", h)
	}

	var data *apiResponse
	if err = json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal("Error unmarshaling JSON body: ", err)
	}

	if data.Status != "success" {
		t.Fatalf("Expected success status, got %q", &data.Status)
	}

	if string(data.Data.Result.Value.String()) != expected {
		t.Fatalf("Expected result %v, got %v", expected, data.Data.Result.Value.String())
	}
}

func queryURL(query string) string {
	queryValues := &url.Values{
		"query": []string{query},
	}
	return fmt.Sprintf("%s%s/query/?%s", baseURL, apiRoute, queryValues.Encode())
}

func waitForServer(u string) error {
	c := make(chan error, 1)
	go func() {
		for {
			_, err := http.Get(u)
			if err == nil {
				c <- nil
				break
			}

			switch err.(type) {
			case *url.Error:
				if strings.HasSuffix(err.Error(), "connection refused") {
					time.Sleep(100 * time.Millisecond)
					continue
				}
			}

			c <- err
		}
	}()

	select {
	case err := <-c:
		return err
	case <-time.After(10 * time.Second):
		return errors.New("timed out wating for server to start")
	}
}
