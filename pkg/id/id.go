//Twitter-Snowflake
//
//1                                               42           52              64
//+-----------------------------------------------+------------+---------------+
//|				timestamp(ms)                     | worker id  | sequence      |
//+-----------------------------------------------+------------+---------------+
//| 0000000000 0000000000 0000000000 0000000000 0 | 0000000000 | 0000000000 00 |
//+-----------------------------------------------+------------+---------------+
//
// 1. 41 bits of timestamp,the value is now()-start time
// 2. 10 bits of mechine
// 3. 12 bits of sequence

package id

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	twepoch        = int64(1514736000000) //init timestamp(2018-01-01)
	workeridBits   = uint(10)             //bits of mechine id
	sequenceBits   = uint(12)             //bits of sequence
	workeridMax    = int64(-1 ^ (-1 << workeridBits))
	sequenceMask   = int64(-1 ^ (-1 << sequenceBits))
	workeridShift  = sequenceBits
	timestampShift = sequenceBits + workeridBits
)

type Snowflake struct {
	sync.Mutex
	timestamp int64
	workerid  int64
	sequence  int64
}

var s *Snowflake

func init() {
	var err error
	//blog is a single sever,set workerid is 0 temporarily
	s, err = newSnowflake(int64(0))
	if err != nil {
		fmt.Println("init id fail. error: ", err)
		os.Exit(1)
	}
}

func newSnowflake(workerid int64) (*Snowflake, error) {
	if workerid < 0 || workerid > workeridMax {
		return nil, errors.New("worerid must be between 0 and " + strconv.FormatInt(workeridMax, 10))
	}
	return &Snowflake{
		timestamp: 0,
		workerid:  workerid,
		sequence:  0,
	}, nil
}

func Generate() int64 {
	s.Lock()
	defer s.Unlock()
	now := time.Now().UnixNano() / 1000000
	if s.timestamp == now {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			for now <= s.timestamp {
				now = time.Now().UnixNano() / 1000000
			}
		}
	} else {
		s.sequence = 0
	}
	s.timestamp = now
	r := int64((now-twepoch)<<timestampShift | (s.workerid << workeridShift) | (s.sequence))
	return r
}
