// Package mail provides local, unauthenticated SMTP delivery to MailHog.
package mail

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"
)

type Sender struct {
	Addr, From, ResetURL string
	Timeout              time.Duration
}

func (s *Sender) SendPasswordReset(ctx context.Context, email, token string) error {
	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if err = client.Mail(s.From); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	link, err := url.Parse(s.ResetURL)
	if err != nil {
		return err
	}
	// Fragment keeps the secret out of HTTP access logs and Referer headers.
	link.Fragment = "token=" + url.QueryEscape(token)
	if strings.ContainsAny(email+s.From, "\r\n") {
		return fmt.Errorf("invalid mail address")
	}
	_, err = fmt.Fprintf(writer, "From: %s\r\nTo: %s\r\nSubject: Reset your Highload Counters password\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nOpen this link to reset your password:\r\n%s\r\n\r\nIf you did not request this, ignore this message.\r\n", s.From, email, link.String())
	if err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
