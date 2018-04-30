// Copyright © 2018 Banco Bilbao Vizcaya Argentaria S.A.  All rights reserved.
// Use of this source code is governed by an Apache 2 License
// that can be found in the LICENSE file

/*
	Package agent implements the command line interface to interact with the
	API rest
*/
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"verifiabledata/api/apihttp"
	"verifiabledata/balloon"
	"verifiabledata/balloon/hashing"
	"verifiabledata/log"
)

type HttpClient struct {
	endpoint string
	apiKey   string
	log      log.Logger
	http.Client
}

func NewHttpClient(endpoint, apiKey string, logger log.Logger) *HttpClient {
	return &HttpClient{
		endpoint,
		apiKey,
		logger,
		*http.DefaultClient,
	}
}

func (c HttpClient) doReq(method, path string, data []byte) ([]byte, error) {
	req, err := http.NewRequest(method, c.endpoint+path, bytes.NewBuffer(data))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.apiKey)

	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	bodyBytes, _ := ioutil.ReadAll(resp.Body)

	return bodyBytes, nil
}

func (c HttpClient) Add(event string) (*apihttp.Snapshot, error) {

	fmt.Println(event)
	data, _ := json.Marshal(&apihttp.Event{[]byte(event)})

	body, err := c.doReq("POST", "/events", data)
	if err != nil {
		return nil, err
	}

	var snapshot apihttp.Snapshot

	json.Unmarshal(body, &snapshot)

	return &snapshot, nil

}

func (c HttpClient) Membership(key []byte, version uint) (*balloon.Proof, error) {

	query, _ := json.Marshal(&apihttp.MembershipQuery{
		key,
		version,
	})
	body, err := c.doReq("POST", "/proofs/membership", query)
	if err != nil {
		return nil, err
	}

	var proof apihttp.MembershipProof

	json.Unmarshal(body, &proof)

	return apihttp.ToBalloonProof("/tmp/balloon.db", &proof, hashing.Sha256Hasher), nil

}