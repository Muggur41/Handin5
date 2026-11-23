# Handin5 – Distributed Auction

An auction system in Go with gRPC. It runs as two server nodes (a primary and a backup) plus a client. The primary handles all bids and copies each one to the backup. If the primary crashes, the client switches to the backup, which takes over. The auction ends after a set time, and the highest bidder wins.

## Requirements

- Go (1.27 or newer)

## How to run

Open three terminals in this folder.

**1. Start the backup first:**
```
go run ./server -port 5002
```

**2. Start the primary:**
```
go run ./server -port 5001 -primary -backup localhost:5002
```

**3. Start a client:**
```
go run ./client -id alice
```

You can start more clients in new terminals with a different `-id` (e.g. `-id bob`).

## Using the client

| Command    | What it does |
|------------|--------------|
| `bid 50`   | Bid 50. Answers `success`, `fail` (too low or auction over) or `exception` (invalid input) |
| `result`   | Shows the highest bid and bidder, or the winner if the auction is over |

## Options

| Flag        | Default            | Meaning |
|-------------|--------------------|---------|
| `-port`     | `5001`             | Port the server listens on |
| `-primary`  | off                | Start this node as the primary |
| `-backup`   | none               | Backup address (primary only) |
| `-duration` | `100s`             | How long the auction runs |
| `-id`       | `alice`            | Bidder name (client) |
| `-servers`  | `localhost:5001,localhost:5002` | Node addresses, primary first (client) |

## Testing a crash

1. Start everything as above and make a few bids.
2. Press Ctrl+C in the primary's terminal.
3. Bid again in the client. It switches to the backup after about 2 seconds, and the earlier bids are still there.
