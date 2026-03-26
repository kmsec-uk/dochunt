package warc

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strconv"

	"github.com/kmsec-uk/ccdocs/gdoc"
)

const capacity = 1024 * 1024

// Variables for parsing the WARC item
var (
	warcHeader                = []byte("WARC/1.0")
	warcIdentifiedPayloadType = []byte("WARC-Identified-Payload-Type:")
	warcDate                  = []byte("WARC-Date:")
	warcContentLength         = []byte("Content-Length:")
	textHtml                  = []byte("text/html")
	newLine                   = byte('\n')
	colon                     = []byte(":")
	separator                 = []byte("\r\n") // separator between header blocks
)

// ParseGZippedWarcGDoc takes an io.ReadCloser (a la response.Body)
// and parses the embedded GDoc HTML. It assumes that there is only
// a single WARC item, which is typically gotten via specifying the
// offset and length.

// Note that this is not a full WARC parser implementation, as we
// are only interested in a specific use-case and specifically for
// CommonCrawl's gzipped data.
func ParseGzippedWarcGDoc(r io.ReadCloser, doc *gdoc.Doc) error {
	defer r.Close()
	reader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer reader.Close()

	br := bufio.NewReaderSize(reader, capacity)
	ln := 0

	line, err := br.ReadSlice(newLine)
	if err != nil {
		return fmt.Errorf("reading line %d: %w", ln, err)
	}
	ln += 1
	if bytes.HasPrefix(line, warcHeader) {
		return fmt.Errorf(
			"did not find WARC header on line 1 -" +
				"possibly corrupted WARC record")
	}
	var contentLength int64
	var gotCaptureDate bool
	var checkedContentType bool
	// walk through WARC headers
	for {
		hdr, err := br.ReadSlice(newLine)
		if err != nil {
			return fmt.Errorf("reading line %d: %w", ln, err)
		}
		ln += 1
		if bytes.Equal(hdr, separator) {
			// end of headers
			break
		}
		if bytes.HasPrefix(hdr, warcContentLength) {
			// is the content-length header
			_, val, ok := bytes.Cut(hdr, colon)
			if !ok {
				return fmt.Errorf("line %d: identified header `%s` but could not get value after `:`", ln, string(warcContentLength))
			}
			contentLength, err = strconv.ParseInt(string(bytes.TrimSpace(val)), 10, 64)
			if err != nil {
				return fmt.Errorf("line %d: content-length header: strconv: %w", ln, err)
			}
			continue
		}
		if bytes.HasPrefix(hdr, warcDate) {
			// fmt.Println(string(hdr))
			_, val, ok := bytes.Cut(hdr, colon)
			if !ok {
				return fmt.Errorf("line %d: identified header `%s` but could not get value after `:`", ln, string(warcDate))
			}
			doc.Timestamp = string(bytes.TrimSpace(val))
			gotCaptureDate = true
			continue
		}
		if bytes.HasPrefix(hdr, warcIdentifiedPayloadType) {
			ct := bytes.TrimSpace(hdr)
			// error out on non text/html response
			if !bytes.HasSuffix(ct, textHtml) {
				return fmt.Errorf("line %d: will not be able to process a non-text/html payload response: %s", ln, string(ct))
			}
			checkedContentType = true
			continue
		}
	}
	if contentLength == 0 || !gotCaptureDate || !checkedContentType {
		return fmt.Errorf("line %d: went through WARC headers but did not identify the content-type, content-length, or capture date.", ln)
	}
	// skip all server headers
	for {
		hdr, err := br.ReadSlice(newLine)
		if err != nil {
			return fmt.Errorf("line %d: error reading: %w", ln, err)
		}
		ln += 1
		// start subtracting contentLenght as WARC content-length *includes* the server headers.
		contentLength -= int64(len(hdr))
		if len(bytes.TrimSpace(hdr)) == 0 {
			// end of headers,
			// nb: note that the behaviour here is slightly different
			// from looking for a specific `separator`,
			// in the remote chance that the server is not compliant.
			break
		}
	}
	// finally, parse the html
	htmlReader := io.LimitReader(br, contentLength)
	err = doc.ParseHtml(htmlReader)
	if err != nil {
		return fmt.Errorf("parsing Google Docs html: %w", err)
	}

	return nil
}
