package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	mediapb "media-service/proto"
)

const apiKey = "secret-media-token"

func main() {
	connection, err := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("connect to server: %v", err)
	}
	defer connection.Close()

	client := mediapb.NewMediaServiceClient(connection)
	fmt.Println("=== Media Processing Service CLI Client ===")
	fmt.Println()

	verifyAuthentication(client)

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-api-key", apiKey))
	fmt.Println("[Step 2] Sending SubmitJob request with valid metadata...")
	job, err := client.SubmitJob(ctx, &mediapb.ProcessMediaRequest{
		ClientId:     "media-cli",
		Filename:     "input_video.mov",
		TargetFormat: mediapb.MediaFormat_MEDIA_FORMAT_MP4,
		QualityLevel: 85,
	})
	if err != nil {
		log.Fatalf("submit job: %v", err)
	}
	fmt.Printf("[SUCCESS] Job created successfully:\n  - Job ID:       %s\n  - Initial State: %s\n  - Submitted At:  %s\n\n", job.GetJobId(), job.GetStatus(), time.Unix(job.GetCreatedAt(), 0).Format(time.RFC3339))

	fmt.Println("[Step 3] Subscribing to Server Streaming: TrackProgress...")
	stream, err := client.TrackProgress(ctx, &mediapb.GetJobStatusRequest{JobId: job.GetJobId()})
	if err != nil {
		log.Fatalf("track progress: %v", err)
	}
	for {
		update, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Fatalf("receive progress: %v", err)
		}
		fmt.Printf("  [%3d%%] %s\n", update.GetProgressPercent(), update.GetCurrentStage())
	}
	fmt.Println()

	fmt.Println("[Step 4] Checking final job status via GetJobStatus RPC...")
	finalStatus, err := client.GetJobStatus(ctx, &mediapb.GetJobStatusRequest{JobId: job.GetJobId()})
	if err != nil {
		log.Fatalf("get final status: %v", err)
	}
	fmt.Printf("  - Current Status: %s\n  - Progress:       %d%%\n  - Completed At:   %s\n\n", finalStatus.GetStatus(), finalStatus.GetProgressPercent(), time.Unix(finalStatus.GetCompletedAt(), 0).Format(time.RFC3339))
	fmt.Println("=== Processing pipeline finished successfully ===")
}

func verifyAuthentication(client mediapb.MediaServiceClient) {
	fmt.Println("[Step 1] Attempting RPC call WITHOUT authentication metadata...")
	_, err := client.SubmitJob(context.Background(), &mediapb.ProcessMediaRequest{})
	if status.Code(err) != codes.Unauthenticated {
		log.Fatalf("expected Unauthenticated error, got %v", err)
	}
	fmt.Printf("[ERROR] Server responded with gRPC error: %v\n", err)
	fmt.Println("[INFO] Authentication enforcement verified successfully.")
	fmt.Println()
}
