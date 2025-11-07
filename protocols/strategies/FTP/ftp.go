package FTP

import (
	"bufio"
	"net"
	"strings"
	"time"

	"github.com/mariocandela/beelzebub/v3/parser"
	"github.com/mariocandela/beelzebub/v3/tracer"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

type FTPStrategy struct {
}

func (ftpStrategy *FTPStrategy) Init(servConf parser.BeelzebubServiceConfiguration, tr tracer.Tracer) error {
	listen, err := net.Listen("tcp", servConf.Address)
	if err != nil {
		log.Errorf("Error during init FTP Protocol: %s", err.Error())
		return err
	}

	go func() {
		for {
			if conn, err := listen.Accept(); err == nil {
				go handleFTPConnection(conn, servConf, tr)
			}
		}
	}()

	log.WithFields(log.Fields{
		"port":   servConf.Address,
		"banner": servConf.Banner,
	}).Infof("Init service %s", servConf.Protocol)
	return nil
}

func handleFTPConnection(conn net.Conn, servConf parser.BeelzebubServiceConfiguration, tr tracer.Tracer) {
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(time.Duration(servConf.DeadlineTimeoutSeconds) * time.Second))

	host, port, _ := net.SplitHostPort(conn.RemoteAddr().String())
	sessionID := uuid.New().String()

	// Send FTP banner
	banner := servConf.Banner
	if banner == "" {
		banner = "220 FTP Server Ready"
	}
	if _, err := conn.Write([]byte(banner + "\r\n")); err != nil {
		return
	}

	// Log initial connection
	tr.TraceEvent(tracer.Event{
		Msg:         "New FTP connection",
		Protocol:    tracer.FTP.String(),
		Status:      tracer.Start.String(),
		RemoteAddr:  conn.RemoteAddr().String(),
		SourceIp:    host,
		SourcePort:  port,
		ID:          sessionID,
		Description: servConf.Description,
	})

	reader := bufio.NewReader(conn)
	username := ""

	// FTP command loop
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		command := strings.TrimSpace(line)
		cmdUpper := strings.ToUpper(command)

		// Log each command
		tr.TraceEvent(tracer.Event{
			Msg:         "FTP Command",
			Protocol:    tracer.FTP.String(),
			Command:     command,
			Status:      tracer.Interaction.String(),
			RemoteAddr:  conn.RemoteAddr().String(),
			SourceIp:    host,
			SourcePort:  port,
			ID:          sessionID,
			Description: servConf.Description,
			User:        username,
		})

		var response string

		// Handle FTP commands
		switch {
		case strings.HasPrefix(cmdUpper, "USER "):
			if len(command) > 5 {
				username = strings.TrimSpace(command[5:])
			}
			response = "331 Password required for " + username + "\r\n"

		case strings.HasPrefix(cmdUpper, "PASS "):
			password := ""
			if len(command) > 5 {
				password = strings.TrimSpace(command[5:])
			}

			// Log login attempt
			tr.TraceEvent(tracer.Event{
				Msg:         "FTP Login Attempt",
				Protocol:    tracer.FTP.String(),
				Command:     command,
				Status:      tracer.Stateless.String(),
				RemoteAddr:  conn.RemoteAddr().String(),
				SourceIp:    host,
				SourcePort:  port,
				ID:          sessionID,
				Description: servConf.Description,
				User:        username,
				Password:    password,
			})

			response = "230 Login successful\r\n"

		case cmdUpper == "SYST":
			response = "215 UNIX Type: L8\r\n"

		case cmdUpper == "PWD":
			response = "257 \"/\" is current directory\r\n"

		case strings.HasPrefix(cmdUpper, "CWD "):
			response = "250 Directory changed successfully\r\n"

		case cmdUpper == "LIST" || strings.HasPrefix(cmdUpper, "LIST "):
			response = "150 Opening ASCII mode data connection for file list\r\n"
			response += "drwxr-xr-x  2 ftp ftp 4096 Jan 01 00:00 files\r\n"
			response += "-rw-r--r--  1 ftp ftp 1234 Jan 01 00:00 readme.txt\r\n"
			response += "226 Transfer complete\r\n"

		case strings.HasPrefix(cmdUpper, "RETR "):
			response = "550 File not found\r\n"

		case strings.HasPrefix(cmdUpper, "STOR "):
			response = "550 Permission denied\r\n"

		case cmdUpper == "PASV":
			response = "227 Entering Passive Mode (127,0,0,1,195,149)\r\n"

		case strings.HasPrefix(cmdUpper, "TYPE "):
			response = "200 Type set to binary\r\n"

		case cmdUpper == "QUIT":
			conn.Write([]byte("221 Goodbye\r\n"))
			return

		default:
			response = "502 Command not implemented\r\n"
		}

		// Write response
		if _, err := conn.Write([]byte(response)); err != nil {
			break
		}
	}

	// Log disconnection
	tr.TraceEvent(tracer.Event{
		Msg:         "FTP connection closed",
		Protocol:    tracer.FTP.String(),
		Status:      tracer.End.String(),
		RemoteAddr:  conn.RemoteAddr().String(),
		SourceIp:    host,
		SourcePort:  port,
		ID:          sessionID,
		Description: servConf.Description,
	})
}
