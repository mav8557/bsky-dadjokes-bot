package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	appbsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/util"

	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
)

var (
	BskyID          string
	BskyAppPassword string
	BskyPDSUrl      string
)

type DadJokeResponse struct {
	Id     string `json:"id"`
	Joke   string `json:"joke"`
	Status uint   `json:"status"`
}

type Joke string

func init() {
	var ok bool
	BskyID, ok = os.LookupEnv("BSKY_ID")
	if !ok {
		log.Fatal("Missing BSKY_ID")
	}
	BskyAppPassword, ok = os.LookupEnv("BSKY_APP_PASSWORD")
	if !ok {
		log.Fatal("Missing BSKY_APP_PASSWORD")
	}
	BskyPDSUrl, ok = os.LookupEnv("BSKY_PDS_URL")
	if !ok {
		log.Fatal("Missing BSKY_PDS_URL")
	}
}

func createBskyClient(ctx context.Context) (*xrpc.Client, error) {
	client := &xrpc.Client{
		Client: http.DefaultClient,
		Host:   BskyPDSUrl,
	}
	session_input := atproto.ServerCreateSession_Input{
		Identifier: BskyID,
		Password:   BskyAppPassword,
	}
	session, err := atproto.ServerCreateSession(ctx, client, &session_input)
	if err != nil {
		return nil, err
	}

	client.Auth = &xrpc.AuthInfo{
		AccessJwt:  session.AccessJwt,
		RefreshJwt: session.RefreshJwt,
		Handle:     session.Handle,
		Did:        session.Did,
	}
	log.Printf("Logged into BlueSky account %s via PDS %s", BskyID, BskyPDSUrl)
	return client, nil
}

func getDadJoke() (Joke, error) {
	var jokeApiResponse DadJokeResponse
	req, err := http.NewRequest("GET", "https://icanhazdadjoke.com/", nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", "bsky-dadjokes-bot (https://github.com/mav8557/bsky-dadjokes-bot)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 200 {
		err = json.NewDecoder(resp.Body).Decode(&jokeApiResponse)
		if err != nil {
			return "", err
		}
	}
	return Joke(jokeApiResponse.Joke), nil
}

func postDadJoke(ctx context.Context, client *xrpc.Client, joke Joke) error {
	// set the date every time for consistency
	// loc is needed for daylight savings time
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now()
	targetTimeUTC := time.Date(
		now.Year(), now.Month(), now.Day(),
		7, 45, 0, 0, loc,
	).UTC()

	post := appbsky.FeedPost{
		CreatedAt: targetTimeUTC.Format(util.ISO8601),
		Text:      string(joke),
	}

	post_input := &atproto.RepoCreateRecord_Input{
		Collection: "app.bsky.feed.post",
		Repo:       client.Auth.Did,
		Record:     &lexutil.LexiconTypeDecoder{Val: &post},
	}

	response, err := atproto.RepoCreateRecord(ctx, client, post_input)
	if err != nil {
		return err
	}
	log.Printf("response: %+v", response)
	return nil
}

func main() {
	var err error
	ctx := context.Background()
	client, err := createBskyClient(ctx)
	if err != nil {
		log.Fatalf("failed to login to BlueSky with BSKY_ID %s - is BSKY_APP_PASSWORD correct and valid? Error: %v", BskyID, err)
	}
	joke, err := getDadJoke()
	if err != nil {
		log.Fatalf("failed to get dad joke: %v", err)
	}
	err = postDadJoke(ctx, client, joke)
	if err != nil {
		log.Fatalf("failed to post dad joke: %v", err)
	}
}

func updateBio(ctx context.Context, client *xrpc.Client) error {
	return nil
	fd, err := os.Open("profile.png")
	if err != nil {
		return err
	}

	output, err := atproto.RepoUploadBlob(ctx, client, fd)
	if err != nil {
		return err
	}

	fd.Close()

	_, err = appbsky.ActorGetProfile(ctx, client, client.Auth.Did)
	if err != nil {
		return err
	}

	// update profile
	desc := "Coming Soon: Dad jokes, once per day. Source code: https://github.com/"
	displayname := "Dad Jokes, Daily"

	newavatar := lexutil.LexBlob{
		Ref:      output.Blob.Ref,
		MimeType: output.Blob.MimeType,
		Size:     output.Blob.Size,
	}

	newprofile := appbsky.ActorProfile{
		Avatar:      &newavatar,
		DisplayName: &displayname,
		Description: &desc,
	}

	// get latest commit
	latestcommit_output, err := atproto.SyncGetLatestCommit(ctx, client, client.Auth.Did)
	if err != nil {
		return err
	}
	log.Println("latest commit", latestcommit_output.Cid)

	rkey := "self"
	validate := true
	profile_input := &atproto.RepoPutRecord_Input{
		Collection: "app.bsky.actor.profile",
		Rkey:       rkey,
		Repo:       client.Auth.Did,
		Validate:   &validate,
		Record:     &lexutil.LexiconTypeDecoder{Val: &newprofile},
	}

	profile_output, err := atproto.RepoPutRecord(ctx, client, profile_input)
	if err != nil {
		return err
	}

	log.Println("%+v\n", profile_output)
	return nil
}
