package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

// SMTPSender envia correos via SMTP.
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	fromName string
	useTLS   bool
}

func NewSMTPSender(host string, port int, username, password, from, fromName string, useTLS bool) (*SMTPSender, error) {
	if strings.TrimSpace(host) == "" {
		return nil, fmt.Errorf("smtp host is required")
	}
	if strings.TrimSpace(from) == "" {
		return nil, fmt.Errorf("smtp from is required")
	}
	if port == 0 {
		port = 587
	}
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		fromName: fromName,
		useTLS:   useTLS,
	}, nil
}

func (s *SMTPSender) SendVerificationOTP(_ context.Context, toEmail string, code string, expiresAt time.Time) error {
	if strings.TrimSpace(toEmail) == "" {
		return fmt.Errorf("to email is required")
	}

	subject := "Verification code"
	body := fmt.Sprintf(
		"Your verification code is %s.\nIt expires at %s UTC.\n",
		code,
		expiresAt.UTC().Format(time.RFC3339),
	)
	msg := buildMessage(s.from, s.fromName, toEmail, subject, body)
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	if s.useTLS {
		conn, err := tls.Dial("tcp", addr, &tls.Config{
			ServerName: s.host,
		})
		if err != nil {
			return err
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.host)
		if err != nil {
			return err
		}
		defer client.Quit()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(s.from); err != nil {
			return err
		}
		if err := client.Rcpt(toEmail); err != nil {
			return err
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write([]byte(msg)); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	}

	return smtp.SendMail(addr, auth, s.from, []string{toEmail}, []byte(msg))
}

func buildMessage(from, fromName, to, subject, body string) string {
	fromHeader := from
	if strings.TrimSpace(fromName) != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, from)
	}

	headers := []string{
		fmt.Sprintf("From: %s", fromHeader),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"UTF-8\"",
	}

	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body
}
