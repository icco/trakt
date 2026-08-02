# trakt

[![Go Reference](https://pkg.go.dev/badge/github.com/icco/trakt.svg)](https://pkg.go.dev/github.com/icco/trakt)
[![Test Go](https://github.com/icco/trakt/actions/workflows/test.yml/badge.svg)](https://github.com/icco/trakt/actions/workflows/test.yml)

A minimal Go client for the [Trakt](https://trakt.tv) API: OAuth device flow, token refresh, and the `sync/*` endpoints.

To use this you need a Trakt API app. Create one at [trakt.tv/oauth/applications](https://trakt.tv/oauth/applications) and set its redirect URI to `urn:ietf:wg:oauth:2.0:oob` if you plan to use the device flow.

```
go get github.com/icco/trakt
```

## Usage

### Authorizing with the device flow

The device flow is the right choice for a headless service: the user approves on another device and you never handle their password.

```go
c := trakt.NewClient(clientID, clientSecret)

dc, err := c.RequestDeviceCode(ctx)
if err != nil {
  return err
}
fmt.Printf("Go to %s and enter %s\n", dc.VerificationURL, dc.UserCode)

// Poll until the user approves. PollForToken returns (nil, nil) while pending.
deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
for time.Now().Before(deadline) {
  time.Sleep(time.Duration(dc.Interval) * time.Second)

  tok, err := c.PollForToken(ctx, dc.DeviceCode)
  if err != nil {
    return err
  }
  if tok != nil {
    save(tok) // tok.ExpiresAt() tells you when to refresh
    break
  }
}
```

### Refreshing

```go
tok, err := c.RefreshToken(ctx, stored.RefreshToken)
```

### Reading sync endpoints

```go
rows, err := c.Sync(ctx, tok.AccessToken, "sync/ratings/movies")
for _, row := range rows {
  // Exactly one of Movie/Show is set, depending on the endpoint.
  if row.Movie != nil {
    fmt.Println(row.Movie.Title, row.Movie.Year, row.Rating, row.Movie.IDs.IMDb)
  }
}
```

Useful paths: `sync/watched/movies`, `sync/watched/shows`, `sync/ratings/movies`, `sync/ratings/shows`, `sync/watchlist/movies`, `sync/watchlist/shows`.

## Notes

- **Read-only, and only the endpoints I needed.** This covers auth plus `sync/*` reads. There is no search, no scrobbling, no list management, no pagination. PRs welcome.
- **`PollForToken` returns `(nil, nil)` while the user has not yet approved.** Trakt signals that state with an HTTP 400, which is the normal case for most of the flow, so treating it as an error would mean aborting on the expected path. A `nil` token with a `nil` error means "keep polling"; any other failure comes back as an error.
- **`Sync` returns rows where exactly one of `Movie`/`Show` is set.** Which one depends on the endpoint you asked for. `Rating` is only meaningful on `sync/ratings/*`.
- **`BaseURL` is exported** so you can point the client at a test server.
- No third-party dependencies.

## License

MIT
