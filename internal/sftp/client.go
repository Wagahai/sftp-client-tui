package sftp

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	gsftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// UnknownHostError is returned by Connect when the server's host key is not in
// known_hosts. Callers should prompt the user, then call AddKnownHost and retry.
type UnknownHostError struct {
	Host        string
	Fingerprint string
	Key         ssh.PublicKey
}

func (e *UnknownHostError) Error() string {
	return fmt.Sprintf("unknown host key for %s (%s)", e.Host, e.Fingerprint)
}

type ConnectOpts struct {
	Host          string
	Port          string
	User          string
	Password      string
	KeyPath       string
	KeyPassphrase string
}

type Client struct {
	SFTP      *gsftp.Client
	SSH       *ssh.Client
	ConnLabel string
}

func Connect(opts ConnectOpts) (*Client, error) {
	if opts.Port == "" {
		opts.Port = "22"
	}

	authMethods := buildAuthMethods(opts)
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method provided")
	}

	// captured is set by the host key callback when the host is unknown,
	// allowing us to return the typed error without relying on SSH library wrapping.
	var captured *UnknownHostError
	hkCb, err := buildHostKeyCallback(&captured)
	if err != nil {
		return nil, fmt.Errorf("loading known_hosts: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            opts.User,
		Auth:            authMethods,
		HostKeyCallback: hkCb,
		Timeout:         15 * time.Second,
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-gcm@openssh.com",
				"aes256-gcm@openssh.com",
				"chacha20-poly1305@openssh.com",
				"aes128-ctr",
				"aes192-ctr",
				"aes256-ctr",
				// Legacy — for compatibility with old servers
				"aes128-cbc",
				"3des-cbc",
				"arcfour256",
				"arcfour128",
				"arcfour",
			},
			KeyExchanges: []string{
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				// Legacy
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
			MACs: []string{
				"hmac-sha2-256-etm@openssh.com",
				"hmac-sha2-512-etm@openssh.com",
				"hmac-sha2-256",
				"hmac-sha1",
				"hmac-sha1-96",
			},
		},
		HostKeyAlgorithms: []string{
			"ecdsa-sha2-nistp256",
			"ecdsa-sha2-nistp384",
			"ecdsa-sha2-nistp521",
			"ssh-ed25519",
			"rsa-sha2-512",
			"rsa-sha2-256",
			"ssh-rsa", // Legacy
		},
	}

	addr := net.JoinHostPort(opts.Host, opts.Port)
	sshClient, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		if captured != nil {
			return nil, captured
		}
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}

	sftpClient, err := gsftp.NewClient(sshClient)
	if err != nil {
		sshClient.Close()
		return nil, fmt.Errorf("sftp handshake: %w", err)
	}

	label := fmt.Sprintf("%s@%s", opts.User, opts.Host)
	return &Client{SFTP: sftpClient, SSH: sshClient, ConnLabel: label}, nil
}

func (c *Client) Close() {
	if c.SFTP != nil {
		c.SFTP.Close()
	}
	if c.SSH != nil {
		c.SSH.Close()
	}
}

// buildHostKeyCallback returns a HostKeyCallback that verifies against
// ~/.ssh/known_hosts. When a host is not found, it sets *captured and returns
// the UnknownHostError so Connect can surface it to the caller.
func buildHostKeyCallback(captured **UnknownHostError) (ssh.HostKeyCallback, error) {
	khPath := knownHostsPath()

	var strictCb ssh.HostKeyCallback
	if _, err := os.Stat(khPath); err == nil {
		cb, err := knownhosts.New(khPath)
		if err != nil {
			return nil, err
		}
		strictCb = cb
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		if strictCb != nil {
			err := strictCb(hostname, remote, key)
			if err == nil {
				return nil
			}
			var ke *knownhosts.KeyError
			if errors.As(err, &ke) && len(ke.Want) == 0 {
				uhe := &UnknownHostError{
					Host:        hostname,
					Fingerprint: ssh.FingerprintSHA256(key),
					Key:         key,
				}
				*captured = uhe
				return uhe
			}
			// Key mismatch — host is known but key changed (possible MITM).
			return err
		}
		// No known_hosts file yet — every host is new.
		uhe := &UnknownHostError{
			Host:        hostname,
			Fingerprint: ssh.FingerprintSHA256(key),
			Key:         key,
		}
		*captured = uhe
		return uhe
	}, nil
}

// AddKnownHost appends a host key entry to ~/.ssh/known_hosts.
func AddKnownHost(host string, key ssh.PublicKey) error {
	khPath := knownHostsPath()
	if err := os.MkdirAll(filepath.Dir(khPath), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(khPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	// knownhosts.Normalize: "host:22" → "host", "host:2222" → "[host]:2222".
	// MarshalAuthorizedKey returns "algo base64key\n".
	_, err = fmt.Fprintf(f, "%s %s", knownhosts.Normalize(host), ssh.MarshalAuthorizedKey(key))
	return err
}

func knownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "known_hosts")
}

func buildAuthMethods(opts ConnectOpts) []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if opts.KeyPath != "" {
		key, err := os.ReadFile(opts.KeyPath)
		if err == nil {
			var signer ssh.Signer
			if opts.KeyPassphrase != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(opts.KeyPassphrase))
			} else {
				signer, err = ssh.ParsePrivateKey(key)
			}
			if err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}

	if opts.Password != "" {
		methods = append(methods, ssh.Password(opts.Password))
		// Also try keyboard-interactive with the same password for servers
		// that require it instead of direct password auth.
		methods = append(methods, ssh.KeyboardInteractive(
			func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = opts.Password
				}
				return answers, nil
			},
		))
	}

	return methods
}
