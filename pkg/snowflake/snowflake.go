package snowflake

import (
	"sync"
	"time"
)

const(
	// Max 22 bits for machineBits + sequenceBits
	// 41 bits - timestamp
	// 1 bit - 0 (making the id positive only)
	epoch = 1767268800
	machineBits = 10
	// timestampBits = 41
	sequenceBits = 12 

	maxMachineId = -1 ^ (-1 << machineBits) // 1023 -> 0-1023
	maxSequence = -1 ^ (-1 << sequenceBits) // 4095 -> 0-4095

	machineShift = sequenceBits
	timestampShift = sequenceBits + machineBits
)

type Snowflake struct{
	mu sync.Mutex
	lastStamp int64
	machineId int64
	sequence int64
}

func New(machineId int64) *Snowflake{
	if machineId < 0 || machineId > maxMachineId{
		panic("invalid machine id")
	}
	return &Snowflake{
		machineId: machineId,
	}
}

// Gets current millisecond
func currMilli() int64{
	return time.Now().UnixMilli()
}

func waitNextMilli(lastMilli int64) int64{
	now := currMilli()
	for now <= lastMilli {
		now = currMilli()
	}
	return now
}

func (s *Snowflake) NextID() int64{
	s.mu.Lock()
	defer s.mu.Unlock()

	now := currMilli()

	// Edge case - what if curr time is less than last time
	if now<s.lastStamp{
		if s.lastStamp - now < 5{ // Tolerate drift upto 5 ms
			now = s.lastStamp
		}else{
			now = waitNextMilli(s.lastStamp) // help recover safely
		}
	}

	if now == s.lastStamp{
		s.sequence = (s.sequence + 1) & maxSequence
		if s.sequence == 0 {
			now = waitNextMilli(s.lastStamp)
		}
	}else{
		s.sequence = 0
	}

	s.lastStamp = now

	id := ((now - epoch)<< timestampShift) |
	 		(s.machineId << machineShift) |
	  		s.sequence

	return id
}