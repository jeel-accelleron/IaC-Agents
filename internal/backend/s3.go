package backend

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/ghcp-iac/ghcp-iac-workflow/internal/stateparser"
)

// S3Backend implements StateBackend for AWS S3
type S3Backend struct {
	config   BackendConfig
	s3Client *s3.Client
}

// NewS3Backend creates a new S3 backend
func NewS3Backend(backendConfig BackendConfig) (*S3Backend, error) {
	if backendConfig.Container == "" {
		return nil, fmt.Errorf("bucket name (container) is required for S3 backend")
	}
	
	if backendConfig.Key == "" {
		return nil, fmt.Errorf("key/path is required for S3 backend")
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, ErrBackendConnection{Backend: "s3", Err: err}
	}

	// Override endpoint if provided in Connection field
	var opts []func(*s3.Options)
	if backendConfig.Connection != "" {
		opts = append(opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(backendConfig.Connection)
		})
	}

	client := s3.NewFromConfig(cfg, opts...)

	return &S3Backend{
		config:   backendConfig,
		s3Client: client,
	}, nil
}

// FetchState retrieves the Terraform state file from S3
func (b *S3Backend) FetchState(ctx context.Context) (*stateparser.StateFile, error) {
	// Get the object from S3
	result, err := b.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.config.Container),
		Key:    aws.String(b.config.Key),
	})
	if err != nil {
		return nil, ErrStateNotFound{
			Backend: "s3",
			Path:    fmt.Sprintf("%s/%s", b.config.Container, b.config.Key),
		}
	}
	defer result.Body.Close()

	// Read the object content
	stateJSON, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object content: %w", err)
	}

	// Parse the state file
	stateFile, err := stateparser.ParseStateFile(stateJSON)
	if err != nil {
		return nil, ErrInvalidState{Err: err}
	}

	return stateFile, nil
}

// ListStateFiles lists all .tfstate files in the S3 bucket
func (b *S3Backend) ListStateFiles(ctx context.Context) ([]string, error) {
	var stateFiles []string
	
	// List objects in the bucket
	paginator := s3.NewListObjectsV2Paginator(b.s3Client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.config.Container),
	})
	
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list S3 objects: %w", err)
		}
		
		for _, obj := range page.Contents {
			if obj.Key != nil {
				// Filter for .tfstate files only
				if len(*obj.Key) > 8 && (*obj.Key)[len(*obj.Key)-8:] == ".tfstate" {
					stateFiles = append(stateFiles, *obj.Key)
				}
			}
		}
	}
	
	return stateFiles, nil
}

// Type returns the backend type
func (b *S3Backend) Type() string {
	return "s3"
}
