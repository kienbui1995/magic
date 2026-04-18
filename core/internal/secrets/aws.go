package secrets

import (
	"context"
	"fmt"
)

// AWSConfig holds connection settings for AWS Secrets Manager.
type AWSConfig struct {
	Region string // AWS_REGION, e.g. "ap-southeast-1"
	Prefix string // MAGIC_AWS_SECRETS_PREFIX, e.g. "magic/prod/"
}

// AWSSecretsManagerProvider is a stub implementation of the AWS Secrets
// Manager backend.
//
// TODO(vendor): import github.com/aws/aws-sdk-go-v2/config and
// github.com/aws/aws-sdk-go-v2/service/secretsmanager, then replace the
// stub with a real GetSecretValue call:
//
//	awscfg, _ := config.LoadDefaultConfig(ctx, config.WithRegion(cfg.Region))
//	client := secretsmanager.NewFromConfig(awscfg)
//	out, err := client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
//	    SecretId: aws.String(cfg.Prefix + name),
//	})
//	return aws.ToString(out.SecretString), err
type AWSSecretsManagerProvider struct {
	cfg AWSConfig
}

// NewAWSSecretsManagerProvider validates config and returns a stub.
// Construction does not dial AWS.
func NewAWSSecretsManagerProvider(cfg AWSConfig) (*AWSSecretsManagerProvider, error) {
	if cfg.Region == "" {
		return nil, fmt.Errorf("aws: AWS_REGION is required")
	}
	return &AWSSecretsManagerProvider{cfg: cfg}, nil
}

// Get is a stub; see package docs and docs/security/secrets.md for the
// implementation skeleton.
func (a *AWSSecretsManagerProvider) Get(_ context.Context, name string) (string, error) {
	return "", fmt.Errorf(
		"%w: aws secrets manager provider is a stub — vendor "+
			"github.com/aws/aws-sdk-go-v2/service/secretsmanager and implement "+
			"AWSSecretsManagerProvider.Get (see docs/security/secrets.md); "+
			"requested secret=%q in region=%s prefix=%q",
		ErrProviderUnavailable, name, a.cfg.Region, a.cfg.Prefix,
	)
}

// Name identifies this provider in logs and health output.
func (a *AWSSecretsManagerProvider) Name() string { return "aws-secrets-manager (stub)" }
