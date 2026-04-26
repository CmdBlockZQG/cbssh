package tunnel

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
)

func handleSOCKS5(local net.Conn, client dialer) {
	defer local.Close()

	header := make([]byte, 2)
	if _, err := io.ReadFull(local, header); err != nil {
		return
	}
	if header[0] != 0x05 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(local, methods); err != nil {
		return
	}
	if _, err := local.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(local, req); err != nil {
		return
	}
	if req[0] != 0x05 || req[1] != 0x01 {
		writeSOCKSReply(local, 0x07)
		return
	}

	host, err := readSOCKSAddress(local, req[3])
	if err != nil {
		writeSOCKSReply(local, 0x08)
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(local, portBytes); err != nil {
		return
	}
	port := int(binary.BigEndian.Uint16(portBytes))
	remote, err := client.Dial("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		writeSOCKSReply(local, 0x05)
		return
	}
	if err := writeSOCKSReply(local, 0x00); err != nil {
		_ = remote.Close()
		return
	}

	done := pipe(local, remote)
	<-done
}

func readSOCKSAddress(r io.Reader, addressType byte) (string, error) {
	switch addressType {
	case 0x01:
		buf := make([]byte, 4)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	case 0x03:
		lenBuf := make([]byte, 1)
		if _, err := io.ReadFull(r, lenBuf); err != nil {
			return "", err
		}
		buf := make([]byte, int(lenBuf[0]))
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	case 0x04:
		buf := make([]byte, 16)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return net.IP(buf).String(), nil
	default:
		return "", fmt.Errorf("unsupported SOCKS address type %d", addressType)
	}
}

func writeSOCKSReply(w io.Writer, code byte) error {
	_, err := w.Write([]byte{0x05, code, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
	return err
}
