package transport

import (
	"log"
	"sync"
)

// link represents all connectivity to a given endpoint addr.
// It manages the physical connection to this endpoint, while
// providing a multiple connection abstraction to the user.
//
type link struct {
	pipelining int
	addr       *Addr
	onclose    func()

	lk         sync.Mutex // protects open
	open       map[connID]*conn

	trigger    Trigger
	swap       *swapConn

	ach        chan *conn
}

func makeLink(a *Addr, g *gobConn, ach chan *conn, onclose func(), pipelining int) *link {
	l := &link{
		pipelining: pipelining,
		addr:       a,
		onclose:    onclose,
		swap:       makeSwapConn(g, pipelining),
		open:       make(map[connID]*conn),
		ach:        ach,
	}
	go l.readLoop()
	return l
}

func (l *link) close() {
	open := l.shutdown()
	for _, c := range open {
		c.Close()
	}
}

func (l *link) shutdown() (conns map[connID]*conn) {
	l.lk.Lock()
	defer l.lk.Unlock()

	if l.swap == nil {
		return nil
	}

	l.onclose()

	l.swap.Close()
	l.swap = nil

	conns, l.open = l.open, nil
	return conns
}

func (l *link) Open() (*conn, error) {
	id := chooseConnID()
	conn, err := l.add(id)
	if err != nil {
		return nil, err
	}
	if err := l.Write(&openMsg{ID: id}); err != nil {
		return nil, err
	}
	return conn, nil
}

func (l *link) add(id connID) (*conn, error) {
	l.lk.Lock()
	defer l.lk.Unlock()

	c := makeConn(id, l)
	if l.open == nil {
		return nil, ErrEnd
	}
	if _, ok := l.open[id]; ok {
		log.Printf("conn id collision")
		return nil, errCollision
	}
	l.open[id] = c
	return c, nil
}

func (l *link) lookup(id connID) *conn {
	l.lk.Lock()
	defer l.lk.Unlock()

	if l.open == nil {
		return nil
	}
	return l.open[id]
}

func (l *link) readLoop() {
	for {

		// Read link msg
		l.lk.Lock()
		swap := l.swap
		l.lk.Unlock()
		if swap == nil {
			return
		}

		m, err := swap.Read()
		if err != nil {
			// XXX // hook reconnect mechanism
			// Corrupt message; close link
			log.Printf("corrupt msg received from %s (%s)", l.addr, err)
			l.close()
			return
		}

		// Demux open/conn msgs
		switch msg := m.(type) {
		case *openMsg:
			c, err := l.add(msg.ID)
			switch err {
			case nil:
				l.ach <- c
			case errCollision:
				log.Printf("connection id collision, unlikely")
			case ErrEnd:
				return
			}

		case *connMsg:
			c := l.lookup(msg.ID)
			if c == nil {
				// Unknown user connection.
				// Usually a late packet, arriving after a local conn.Close
				continue  // Drop
			}
			c.sendRead(msg.Payload)

		default:
			// Drop unknown messages for forward compatibility
		}
	}
}

func (l *link) Write(payload interface{}) error {
	l.lk.Lock()
	swap := l.swap
	l.lk.Unlock()

	if swap == nil {
		return ErrAlreadyClosed
	}
	if err := swap.Write(payload); err != nil {
		// XXX // hook reconnect mechanism
		l.close()
		return err
	}
	return nil
}

func (l *link) drop(id connID) {
	l.lk.Lock()
	defer l.lk.Unlock()
	if l.open != nil {
		delete(l.open, id)
	}
}

func (l *link) reconnect() {
	// Nop for now
	if !l.trigger.Lock() {
		return
	}
	defer l.trigger.Unlock()
	
	return
}

func (l *link) acceptReconnect(g *gobConn) error {
	return ErrNotSupported
}
