package grpcclient

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	forwarder "codec-svr/proto"
)

type GRPCClient struct {
	conn   *grpc.ClientConn
	client forwarder.ForwarderClient
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	c := forwarder.NewForwarderClient(conn)
	return &GRPCClient{conn: conn, client: c}, nil
}

func (g *GRPCClient) Close() {
	g.conn.Close()
}

func (g *GRPCClient) SendData(deviceID, payload string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &forwarder.DataRequest{
		DeviceId: deviceID,
		Payload:  payload,
	}

	res, err := g.client.SendData(ctx, req)
	if err != nil {
		return err
	}

	if !res.Success {
		log.Printf("Forwarder: failed to send data for device %s", deviceID)
	}

	return nil
}
