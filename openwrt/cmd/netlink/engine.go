package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Iface is one WAN line. Device is used with SO_BINDTODEVICE; Source (optional) is the local IP.
type Iface struct {
	Name    string `json:"name"`
	Device  string `json:"device"`
	Source  string `json:"source,omitempty"`
	Streams int    `json:"streams"`
}

func clientFor(ifc Iface) *http.Client {
	d := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	if ifc.Source != "" {
		d.LocalAddr = &net.TCPAddr{IP: net.ParseIP(ifc.Source)}
	}
	if ifc.Device != "" {
		dev := ifc.Device
		d.Control = func(network, address string, c syscall.RawConn) error {
			var serr error
			err := c.Control(func(fd uintptr) { serr = bindToDevice(int(fd), dev) })
			if err != nil {
				return err
			}
			return serr
		}
	}
	tr := &http.Transport{DialContext: d.DialContext, MaxIdleConnsPerHost: 64,
		TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression: true, ForceAttemptHTTP2: false}
	return &http.Client{Transport: tr}
}

type Block struct {
	Start, End int64 // inclusive
	Done       int64
	state      int32 // 0 pending 1 active 2 done
	hedges     int32
}

type Job struct {
	ID, URL, Path string
	Total         int64
	Ranges        bool
	Ifaces        []Iface
	PerIface      []int64
	Blocks        []*Block
	Err           string
	Status        string // probing, running, done, failed, cancelled
	Started       time.Time
	Finished      time.Time
	cancel        context.CancelFunc
	mu            sync.Mutex
}

func (j *Job) Downloaded() int64 {
	var n int64
	for _, b := range j.Blocks {
		n += atomic.LoadInt64(&b.Done)
	}
	return n
}

const MiB = 1 << 20

func planBlocks(total int64, nIfaces, streams int) []*Block {
	// Same rule as Netlink plan.ts: ~2 blocks per stream, clamped to 1..8 MiB.
	target := int64(nIfaces * streams * 2 * 2)
	bs := (total + target - 1) / target
	if bs < MiB {
		bs = MiB
	}
	if bs > 8*MiB {
		bs = 8 * MiB
	}
	var out []*Block
	for s := int64(0); s < total; s += bs {
		e := s + bs - 1
		if e >= total {
			e = total - 1
		}
		out = append(out, &Block{Start: s, End: e})
	}
	return out
}

func probe(ctx context.Context, c *http.Client, url string) (int64, bool, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "netlink-openwrt/0.1")
	res, err := c.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer res.Body.Close()
	if res.StatusCode == 206 {
		cr := res.Header.Get("Content-Range")
		if i := strings.LastIndex(cr, "/"); i >= 0 {
			if n, err := strconv.ParseInt(cr[i+1:], 10, 64); err == nil {
				return n, true, nil
			}
		}
		return 0, false, nil
	}
	if res.StatusCode == 200 {
		return res.ContentLength, false, nil
	}
	return 0, false, fmt.Errorf("probe: HTTP %d", res.StatusCode)
}

// Run downloads the job. Each interface runs Streams workers pulling pending blocks;
// once none are pending, idle workers race (hedge) the slowest active block.
func (j *Job) Run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	j.cancel = cancel
	defer cancel()
	j.Started = time.Now()
	j.setStatus("probing")
	clients := make([]*http.Client, len(j.Ifaces))
	for i, ifc := range j.Ifaces {
		clients[i] = clientFor(ifc)
	}
	var total int64 = -1
	var ranges bool
	var perr error
	for _, c := range clients { // first interface that answers
		total, ranges, perr = probe(ctx, c, j.URL)
		if perr == nil {
			break
		}
	}
	if perr != nil {
		return j.fail(perr)
	}
	j.Total, j.Ranges = total, ranges
	j.PerIface = make([]int64, len(j.Ifaces))
	part := j.Path + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return j.fail(err)
	}
	defer f.Close()
	if !ranges || total <= 0 {
		j.Blocks = []*Block{{Start: 0, End: -1}}
		j.setStatus("running")
		if err := j.fetch(ctx, clients[0], 0, j.Blocks[0], f, false); err != nil {
			return j.fail(err)
		}
	} else {
		if err := f.Truncate(total); err != nil {
			return j.fail(err)
		}
		streams := 4
		for _, ifc := range j.Ifaces {
			if ifc.Streams > streams {
				streams = ifc.Streams
			}
		}
		j.Blocks = planBlocks(total, len(j.Ifaces), streams)
		j.setStatus("running")
		var wg sync.WaitGroup
		errs := make(chan error, 64)
		for i := range j.Ifaces {
			n := j.Ifaces[i].Streams
			if n <= 0 {
				n = 4
			}
			for s := 0; s < n; s++ {
				wg.Add(1)
				go func(i int) { defer wg.Done(); j.worker(ctx, clients[i], i, f, errs) }(i)
			}
		}
		wg.Wait()
		for _, b := range j.Blocks {
			if atomic.LoadInt32(&b.state) != 2 {
				var e error = errors.New("incomplete")
				select {
				case e = <-errs:
				default:
				}
				if ctx.Err() != nil {
					e = ctx.Err()
				}
				return j.fail(e)
			}
		}
	}
	if err := f.Sync(); err != nil {
		return j.fail(err)
	}
	f.Close()
	if err := os.Rename(part, j.Path); err != nil {
		return j.fail(err)
	}
	j.Finished = time.Now()
	j.setStatus("done")
	return nil
}

func (j *Job) setStatus(s string) { j.mu.Lock(); j.Status = s; j.mu.Unlock() }
func (j *Job) fail(e error) error {
	j.mu.Lock()
	if j.Status != "cancelled" {
		j.Status = "failed"
	}
	j.Err = e.Error()
	j.Finished = time.Now()
	j.mu.Unlock()
	return e
}

func (j *Job) worker(ctx context.Context, c *http.Client, i int, f *os.File, errs chan error) {
	fails := 0
	for ctx.Err() == nil {
		b, hedge := j.claim()
		if b == nil {
			return
		}
		err := j.fetch(ctx, c, i, b, f, hedge)
		if err != nil && !hedge {
			atomic.CompareAndSwapInt32(&b.state, 1, 0) // back to the queue
			fails++
			if fails >= 5 {
				select {
				case errs <- fmt.Errorf("%s: %v", j.Ifaces[i].Name, err):
				default:
				}
				return // this line gives up; others keep going
			}
			time.Sleep(time.Duration(fails) * time.Second)
		} else if err == nil {
			fails = 0
		}
	}
}

func (j *Job) claim() (*Block, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var slow *Block
	var most int64
	for _, b := range j.Blocks {
		if atomic.CompareAndSwapInt32(&b.state, 0, 1) {
			return b, false
		}
		if atomic.LoadInt32(&b.state) == 1 {
			if left := b.End - b.Start + 1 - atomic.LoadInt64(&b.Done); left > most && left > (b.End-b.Start+1)/2 && b.hedges < 1 {
				most, slow = left, b
			}
			if false {
				if left := b.End - b.Start + 1 - atomic.LoadInt64(&b.Done); left > most {
					most, slow = left, b
				}
			}
		}
	}
	if slow != nil && most > 256*1024 {
		slow.hedges++
		return slow, true
	}
	return nil, false
}

// fetch fetches the rest of a block. A hedge refetches from the block's current offset into a
// private counter and only commits when it finishes first.
func (j *Job) fetch(ctx context.Context, c *http.Client, i int, b *Block, f *os.File, hedge bool) error {
	start := b.Start + atomic.LoadInt64(&b.Done)
	req, _ := http.NewRequestWithContext(ctx, "GET", j.URL, nil)
	req.Header.Set("User-Agent", "netlink-openwrt/0.1")
	if b.End >= 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, b.End))
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if b.End >= 0 && res.StatusCode != 206 {
		return fmt.Errorf("HTTP %d for range", res.StatusCode)
	}
	if b.End < 0 && res.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if b.End >= 0 {
		cr := res.Header.Get("Content-Range")
		if !strings.HasPrefix(cr, fmt.Sprintf("bytes %d-", start)) {
			return fmt.Errorf("bad Content-Range %q", cr)
		}
	}
	buf := make([]byte, 64*1024)
	off := start
	for {
		if atomic.LoadInt32(&b.state) == 2 {
			return nil // someone else finished it
		}
		n, rerr := res.Body.Read(buf)
		if n > 0 {
			if b.End >= 0 && off+int64(n) > b.End+1 {
				n = int(b.End + 1 - off)
			}
			if _, err := f.WriteAt(buf[:n], off); err != nil {
				return err
			}
			off += int64(n)
			atomic.AddInt64(&j.PerIface[i], int64(n))
			// Progress only moves forward; a hedge that's ahead advances it too (bytes identical).
			for {
				cur := atomic.LoadInt64(&b.Done)
				if off-b.Start <= cur || atomic.CompareAndSwapInt64(&b.Done, cur, off-b.Start) {
					break
				}
			}
		}
		if rerr == io.EOF {
			if b.End >= 0 && off != b.End+1 {
				return io.ErrUnexpectedEOF
			}
			atomic.StoreInt32(&b.state, 2)
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}
