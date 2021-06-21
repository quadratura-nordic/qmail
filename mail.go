package qmail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/smtp"
	"strings"

	"github.com/google/uuid"
)

type Address struct {
	Email string
	Name  string
}

type inline struct {
	attachment []byte
	uuid       string
}

type Message struct {
	From        Address
	To          []Address
	Cc          []Address
	Bcc         []Address
	Subject     string
	Body        io.Reader
	attachments map[string][]byte
	inlines     map[string]inline
}

func (m *Message) Attach(name string, reader io.Reader) error {
	if m.attachments == nil {
		m.attachments = make(map[string][]byte)
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	m.attachments[name] = b
	return nil
}

func (m *Message) InlineAttach(name string, reader io.Reader) (string, error) {
	if m.inlines == nil {
		m.inlines = make(map[string]inline)
	}

	uuid.NewUUID()
	v1, err := uuid.NewUUID()
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	m.inlines[name] = inline{attachment: b, uuid: v1.String()}
	return v1.String(), nil
}

func (m *Message) bytes() ([]byte, error) {
	v1, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}
	boundary := v1.String()

	var buf bytes.Buffer
	if m.From.Email != "" {
		buf.WriteString("From:")
		if m.From.Name != "" {
			buf.WriteString(m.From.Name + " <" + m.From.Email + ">")
		} else {
			buf.WriteString(m.From.Email)
		}
		buf.WriteRune('\n')
	}

	to := make([]string, len(m.To))
	for i, address := range m.To {
		if address.Name != "" {
			to[i] = address.Name + " <" + address.Email + ">"
		} else {
			to[i] = address.Email
		}
	}

	cc := make([]string, len(m.Cc))
	for i, address := range m.Cc {
		if address.Name != "" {
			cc[i] = address.Name + " <" + address.Email + ">"
		} else {
			cc[i] = address.Email
		}
	}

	if len(to) > 0 {
		buf.WriteString("To: ")
		buf.WriteString(strings.Join(to, ";"))
		buf.WriteRune('\n')
	}

	if len(cc) > 0 {
		buf.WriteString("Cc: ")
		buf.WriteString(strings.Join(cc, ";"))
		buf.WriteRune('\n')
	}

	buf.WriteString("Subject: ")
	buf.WriteString(m.Subject)
	buf.WriteString("\n")
	buf.WriteString("MIME-Version: 1.0\n")

	if len(m.attachments) > 0 || len(m.inlines) > 0 {
		buf.WriteString("Content-Type: multipart/mixed; boundary=")
		buf.WriteString(boundary)
		buf.WriteString("\n")
		buf.WriteString("--")
		buf.WriteString(boundary)
		buf.WriteString("\n")
	}

	buf.WriteString("Content-Type: text/html; charset=utf-8\n")
	if m.Body != nil {
		buf.ReadFrom(m.Body)
	}

	if len(m.attachments) > 0 || len(m.inlines) > 0 {
		for k, v := range m.attachments {
			buf.WriteString("\n\n--")
			buf.WriteString(boundary)
			buf.WriteString("\n")
			buf.WriteString("Content-Type: application/octet-stream\n")
			buf.WriteString("Content-Transfer-Encoding: base64\n")
			buf.WriteString("Content-Disposition: attachment; filename=\"")
			buf.WriteString(k)
			buf.WriteString("\"\n\n")

			b := make([]byte, base64.StdEncoding.EncodedLen(len(v)))
			base64.StdEncoding.Encode(b, v)
			buf.Write(b)
			buf.WriteString("\n--")
			buf.WriteString(boundary)
		}

		for k, v := range m.inlines {
			buf.WriteString("\n\n--")
			buf.WriteString(boundary)
			buf.WriteString("\n")
			buf.WriteString("Content-Type: application/octet-stream\n")
			buf.WriteString("Content-Transfer-Encoding: base64\n")
			buf.WriteString("Content-ID: <")
			buf.WriteString(v.uuid)
			buf.WriteString(">\n")
			buf.WriteString("Content-Disposition: inline; filename=\"")
			buf.WriteString(k)
			buf.WriteString("\"\n\n")

			b := make([]byte, base64.StdEncoding.EncodedLen(len(v.attachment)))
			base64.StdEncoding.Encode(b, v.attachment)
			buf.Write(b)
			buf.WriteString("\n--")
			buf.WriteString(boundary)
		}

		buf.WriteString("--")
	}

	return buf.Bytes(), nil
}

// addr: net.JoinHostPort(host, port)
// auth: smtp.PlainAuth("", username, password, host)
func Send(addr string, auth smtp.Auth, m Message) error {
	if m.From.Email == "" {
		return fmt.Errorf("from is not set")
	}

	receivers := make([]string, len(m.To)+len(m.Cc)+len(m.Bcc))
	c := 0
	for _, address := range m.To {
		receivers[c] = address.Email
		c++
	}
	for _, address := range m.Cc {
		receivers[c] = address.Email
		c++
	}
	for _, address := range m.Bcc {
		receivers[c] = address.Email
		c++
	}

	if c == 0 {
		return fmt.Errorf("zero receivers")
	}

	b, err := m.bytes()
	if err != nil {
		return err
	}

	return smtp.SendMail(addr, auth, m.From.Email, receivers, b)
}
