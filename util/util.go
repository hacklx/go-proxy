package util

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"encoding/base64"
	"time"
	"math/rand"
	"github.com/juju/ratelimit"
)

const S_token = "#PROXY#"

const (
	TOKEN_LEN       = 4
	C2P_CONNECT     = "C2P0"
	C2P_SESSION     = "C2P1"
	C2P_KEEP_ALIVE  = "C2P2"
	P2C_NEW_SESSION = "P2C1"
	SEPS            = "\n"
)

const MAX_STRING = 10240

const base64Table = "lmnopqrstuvwxyzabcdefghijkABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/"


var coder = base64.NewEncoding(base64Table)

func Base64Encode(src []byte) string{
	return coder.EncodeToString(src)
}

func Base64Decode(src string) string{
	decode,_:=coder.DecodeString(src)
	return string(decode)
}

func RandPort(min, max int32) int32 {
	if min >= max || min == 0 || max == 0 {
		return max
	}

	rand.Seed(int64(time.Now().Nanosecond()))

	return rand.Int31n(max-min) + min
}

func Usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
}

func Conn2Str(conn net.Conn) string {
	return conn.LocalAddr().String() + " <-> " + conn.RemoteAddr().String()
}

func CopyFromTo(r, w io.ReadWriteCloser, buf []byte) {
	defer CloseConn(r)
	if buf != nil && len(buf) > 0 {
		_, err := w.Write(buf)
		if err != nil {
			return
		}
	}
	io.Copy(r, w)
}

func CopyRateTo(src,dst io.ReadWriteCloser,maxbit int)(written int64, err error){

	defer CloseConn(src)
	bufer := make([]byte, 32*1024)

	bucket := ratelimit.NewBucketWithRate(float64(maxbit * 1024), int64(maxbit * 1024))
	lr := ratelimit.Reader(dst, bucket)
	lw := ratelimit.Writer(src, bucket)

	for {
		nr, er := lr.Read(bufer)
		if nr > 0 {
			nw, ew := lw.Write(bufer[0:nr])
			if nw > 0 {
				written += int64(nw)
			}
			if ew != nil {
				err = ew
				break
			}
			if nr != nw {
				err = io.ErrShortWrite
				break
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			err = er
			break
		}
	}
	return written, err
}

func CloseConn(a io.ReadWriteCloser) {
	a.Close()
}

func WriteString(w io.Writer, str string) (int, error) {
	binary.Write(w, binary.LittleEndian, int32(len(str)))
	return w.Write([]byte(str))
}

func ReadString(r io.Reader) (string, error) {
	var size int32
	err := binary.Read(r, binary.LittleEndian, &size)
	if err != nil {
		return "", err
	}
	if size > MAX_STRING {
		return "", errors.New("too long string")
	}

	buff := make([]byte, size)
	n, err := r.Read(buff)
	if err != nil {
		return "", err
	}
	if int32(n) != size {
		return "", errors.New("invalid string size")
	}
	return string(buff), nil
}
