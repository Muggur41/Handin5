package main

import (
	"context"
	"flag"
	"log"
	"net"
	"sync"
	"time"

	"Handin5/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Server is one auction node. It is either the primary (handles client
// requests and copies every change to the backup) or the backup (receives
// copies and takes over if the primary crashes).
type Server struct {
	proto.UnimplementedAuctionServer
	proto.UnimplementedReplicationServer
	mu sync.Mutex

	HighestBid    int32
	HighestBidder string
	Bidders       map[string]int32 // bidder -> their last bid (first bid registers them)
	EndTime       time.Time

	isPrimary bool
	backup    proto.ReplicationClient // nil if there is no (live) backup
}

func (s *Server) isOver() bool {
	return time.Now().After(s.EndTime)
}

// Bid is called by clients.
func (s *Server) Bid(ctx context.Context, req *proto.BidRequest) (*proto.BidResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Received bid from %s: %d", req.BidderId, req.BidAmount)

	// Clients only talk to the backup when the primary has crashed,
	// so a client request at the backup means: take over as primary.
	if !s.isPrimary {
		log.Printf("Client contacted backup -> primary must be down, I am now the primary")
		s.isPrimary = true
		s.backup = nil
	}

	if req.BidderId == "" || req.BidAmount <= 0 {
		return &proto.BidResponse{Status: "exception"}, nil
	}
	if s.isOver() {
		return &proto.BidResponse{Status: "fail"}, nil
	}
	// Must beat the current highest bid (which also means beating
	// the bidder's own previous bids).
	if req.BidAmount <= s.HighestBid {
		return &proto.BidResponse{Status: "fail"}, nil
	}

	if _, known := s.Bidders[req.BidderId]; !known {
		log.Printf("Registered new bidder %s", req.BidderId)
	}
	s.Bidders[req.BidderId] = req.BidAmount
	s.HighestBid = req.BidAmount
	s.HighestBidder = req.BidderId

	// Copy the new state to the backup BEFORE answering the client.
	s.replicate()

	log.Printf("Bid accepted. Highest: %s with %d", s.HighestBidder, s.HighestBid)
	return &proto.BidResponse{Status: "success"}, nil
}

// Result is called by clients.
func (s *Server) Result(ctx context.Context, req *proto.ResultRequest) (*proto.ResultResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isPrimary {
		log.Printf("Client contacted backup -> primary must be down, I am now the primary")
		s.isPrimary = true
		s.backup = nil
	}

	return &proto.ResultResponse{
		IsOver:     s.isOver(),
		Winner:     s.HighestBidder,
		HighestBid: s.HighestBid,
	}, nil
}

// replicate sends the current state to the backup. Caller holds s.mu.
// If the backup does not answer within the time limit we assume it has
// crashed and continue alone (we only need to survive one crash).
func (s *Server) replicate() {
	if s.backup == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := s.backup.Replicate(ctx, &proto.ReplicateRequest{
		HighestBidder: s.HighestBidder,
		HighestBid:    s.HighestBid,
		IsOver:        s.isOver(),
		EndTime:       s.EndTime.UnixMilli(),
	})
	if err != nil {
		log.Printf("Backup did not answer (%v) -> assuming it crashed, continuing alone", err)
		s.backup = nil
	}
}

// Replicate is called by the primary on the backup.
func (s *Server) Replicate(ctx context.Context, req *proto.ReplicateRequest) (*proto.ReplicateResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	log.Printf("Replicated from primary: %s with %d", req.HighestBidder, req.HighestBid)
	s.HighestBid = req.HighestBid
	s.HighestBidder = req.HighestBidder
	if req.HighestBidder != "" {
		s.Bidders[req.HighestBidder] = req.HighestBid
	}
	s.EndTime = time.UnixMilli(req.EndTime) // use the primary's end time

	return &proto.ReplicateResponse{Status: "success"}, nil
}

func main() {
	port := flag.String("port", "5001", "port to listen on")
	primary := flag.Bool("primary", false, "start as primary")
	backupAddr := flag.String("backup", "", "address of the backup (only for primary), e.g. localhost:5002")
	duration := flag.Duration("duration", 100*time.Second, "how long the auction runs")
	flag.Parse()

	s := &Server{
		Bidders:   map[string]int32{},
		EndTime:   time.Now().Add(*duration),
		isPrimary: *primary,
	}

	if *primary && *backupAddr != "" {
		conn, err := grpc.NewClient(*backupAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("could not create backup client: %v", err)
		}
		s.backup = proto.NewReplicationClient(conn)
	}

	lis, err := net.Listen("tcp", ":"+*port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	proto.RegisterAuctionServer(grpcServer, s)
	proto.RegisterReplicationServer(grpcServer, s)

	role := "backup"
	if *primary {
		role = "primary"
	}
	log.Printf("Node started as %s on port %s, auction ends at %s", role, *port, s.EndTime.Format("15:04:05"))

	// Primary sends its start state (incl. end time) to the backup,
	// so both nodes agree on when the auction ends.
	if *primary && s.backup != nil {
		s.mu.Lock()
		s.replicate()
		s.mu.Unlock()
	}

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}
