package main

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	mediapb "media-service/proto"
)

const apiKey = "secret-media-token"

type job struct {
	id              string
	filename        string
	targetFormat    mediapb.MediaFormat
	status          string
	progressPercent int32
	createdAt       int64
	completedAt     int64
}

type mediaServer struct {
	mediapb.UnimplementedMediaServiceServer
	mu   sync.RWMutex
	jobs map[string]*job
}

func newMediaServer() *mediaServer {
	return &mediaServer{jobs: make(map[string]*job)}
}

func (s *mediaServer) SubmitJob(_ context.Context, request *mediapb.ProcessMediaRequest) (*mediapb.JobResponse, error) {
	if request.GetClientId() == "" || request.GetFilename() == "" {
		return nil, status.Error(codes.InvalidArgument, "client_id and filename are required")
	}
	if request.GetTargetFormat() == mediapb.MediaFormat_MEDIA_FORMAT_UNSPECIFIED {
		return nil, status.Error(codes.InvalidArgument, "target_format is required")
	}
	if request.GetQualityLevel() < 1 || request.GetQualityLevel() > 100 {
		return nil, status.Error(codes.InvalidArgument, "quality_level must be between 1 and 100")
	}

	createdAt := time.Now().Unix()
	registered := &job{
		id:           uuid.NewString(),
		filename:     request.GetFilename(),
		targetFormat: request.GetTargetFormat(),
		status:       "QUEUED",
		createdAt:    createdAt,
	}

	s.mu.Lock()
	s.jobs[registered.id] = registered
	s.mu.Unlock()

	log.Printf("[STORAGE] Job registered: ID=%s Filename=%s TargetFormat=%s Status=%s", registered.id, registered.filename, registered.targetFormat, registered.status)

	return &mediapb.JobResponse{
		JobId:     registered.id,
		Status:    registered.status,
		CreatedAt: registered.createdAt,
	}, nil
}

func (s *mediaServer) GetJobStatus(_ context.Context, request *mediapb.GetJobStatusRequest) (*mediapb.JobStatusResponse, error) {
	registered, err := s.findJob(request.GetJobId())
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	response := &mediapb.JobStatusResponse{
		JobId:           registered.id,
		Status:          registered.status,
		ProgressPercent: registered.progressPercent,
		CompletedAt:     registered.completedAt,
	}
	s.mu.RUnlock()

	return response, nil
}

func (s *mediaServer) TrackProgress(request *mediapb.GetJobStatusRequest, stream grpc.ServerStreamingServer[mediapb.ProgressUpdate]) error {
	registered, err := s.findJob(request.GetJobId())
	if err != nil {
		return err
	}

	log.Printf("[STREAM] Client connected to track progress for JobID=%s", registered.id)
	steps := []struct {
		stage    string
		progress int32
	}{
		{"Reading media headers", 25},
		{"Transcoding media stream", 50},
		{"Optimizing output size", 75},
		{"Processing completed", 100},
	}

	for _, step := range steps {
		select {
		case <-stream.Context().Done():
			return status.Error(codes.Canceled, "client canceled progress tracking")
		case <-time.After(300 * time.Millisecond):
		}

		if err := stream.Send(&mediapb.ProgressUpdate{
			JobId:           registered.id,
			CurrentStage:    step.stage,
			ProgressPercent: step.progress,
			IsCompleted:     step.progress == 100,
		}); err != nil {
			return err
		}

		s.updateProgress(registered.id, step.progress)
		log.Printf("[STREAM] JobID=%s -> Sent stage %q (%d%%)", registered.id, step.stage, step.progress)
	}

	log.Printf("[STREAM] Progress tracking finished for JobID=%s", registered.id)
	return nil
}

func (s *mediaServer) findJob(id string) (*job, error) {
	s.mu.RLock()
	registered, exists := s.jobs[id]
	s.mu.RUnlock()
	if !exists {
		return nil, status.Errorf(codes.NotFound, "job %q not found", id)
	}
	return registered, nil
}

func (s *mediaServer) updateProgress(id string, progress int32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	registered := s.jobs[id]
	registered.progressPercent = progress
	if progress == 100 {
		registered.status = "COMPLETED"
		registered.completedAt = time.Now().Unix()
		return
	}
	registered.status = "PROCESSING"
}

func unaryInterceptor(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	startedAt := time.Now()
	clientAddress := "unknown"
	if client, ok := peer.FromContext(ctx); ok {
		clientAddress = client.Addr.String()
	}

	metadataValues, exists := metadata.FromIncomingContext(ctx)
	validKey := exists && len(metadataValues.Get("x-api-key")) > 0 && metadataValues.Get("x-api-key")[0] == apiKey
	if !validKey {
		err := status.Error(codes.Unauthenticated, "Invalid or missing API key")
		log.Printf("[INTERCEPTOR] Unary call Method=%s Client=%s Auth=REJECTED Status=%s Duration=%s", info.FullMethod, clientAddress, status.Code(err), time.Since(startedAt))
		return nil, err
	}

	response, err := handler(ctx, request)
	log.Printf("[INTERCEPTOR] Unary call Method=%s Client=%s Auth=SUCCESS Status=%s Duration=%s", info.FullMethod, clientAddress, status.Code(err), time.Since(startedAt))
	return response, err
}

func main() {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("listen on :50051: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(unaryInterceptor))
	mediapb.RegisterMediaServiceServer(grpcServer, newMediaServer())
	log.Println("[SERVER] Starting Media Processing gRPC service on :50051...")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("serve gRPC: %v", err)
	}
}
