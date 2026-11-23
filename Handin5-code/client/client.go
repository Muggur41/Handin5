package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"Handin5/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// The client knows all nodes. It always talks to the first one (the primary).
// If that node does not answer within the time limit, it assumes the node
// crashed and switches to the next one (the backup) for good.
var servers []proto.AuctionClient
var current = 0

func connect(addrs []string) {
	for _, addr := range addrs {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("could not connect to %s: %v", addr, err)
		}
		servers = append(servers, proto.NewAuctionClient(conn))
	}
}

func bid(id string, amount int32) {
	for current < len(servers) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := servers[current].Bid(ctx, &proto.BidRequest{BidderId: id, BidAmount: amount})
		cancel()
		if err == nil {
			fmt.Println("Bid", amount, "->", resp.Status)
			return
		}
		log.Printf("Node %d did not answer, switching to next node", current)
		current++
	}
	fmt.Println("All nodes are down")
}

func result() {
	for current < len(servers) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		resp, err := servers[current].Result(ctx, &proto.ResultRequest{})
		cancel()
		if err == nil {
			if resp.IsOver {
				fmt.Printf("Auction is over. Winner: %s with %d\n", resp.Winner, resp.HighestBid)
			} else {
				fmt.Printf("Auction running. Highest bid: %d by %s\n", resp.HighestBid, resp.Winner)
			}
			return
		}
		log.Printf("Node %d did not answer, switching to next node", current)
		current++
	}
	fmt.Println("All nodes are down")
}

func main() {
	id := flag.String("id", "alice", "your bidder name")
	addrs := flag.String("servers", "localhost:5001,localhost:5002", "nodes, primary first")
	flag.Parse()

	connect(strings.Split(*addrs, ","))
	fmt.Println("Commands: 'bid <amount>' or 'result'")

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 0 {
			continue
		}
		switch parts[0] {
		case "bid":
			if len(parts) != 2 {
				fmt.Println("usage: bid <amount>")
				continue
			}
			amount, err := strconv.Atoi(parts[1])
			if err != nil {
				fmt.Println("amount must be a number")
				continue
			}
			bid(*id, int32(amount))
		case "result":
			result()
		default:
			fmt.Println("Commands: 'bid <amount>' or 'result'")
		}
	}
}
