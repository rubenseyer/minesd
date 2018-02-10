package main

import "log"

type MinesweeperMode string
type MinesweeperDifficulty string

const (
	Solo   = MinesweeperMode("solo")
	Coop   = MinesweeperMode("coop")
	Race   = MinesweeperMode("race")
	Attack = MinesweeperMode("attack")

	Beginner     = MinesweeperDifficulty("beg")
	Intermediate = MinesweeperDifficulty("int")
	Expert       = MinesweeperDifficulty("exp")
	Extreme      = MinesweeperDifficulty("ext")
	Custom       = MinesweeperDifficulty("cus")
)

type Record struct {
	Username   string
	RecordMessage
}

func (s *Server) handleRecord(c *Client, m *RecordMessage) {
	if m.Mode != Solo {
		// TODO maybe store more later
		// TODO maybe broadcast custom games
		return
	}
	r := &Record{Username: c.Username, RecordMessage: *m}
	s.recordsLatest.Value = r
	s.recordsLatest = s.recordsLatest.Next()
	rs := &RecordSyncMessage{Latest: []*Record{r}}
	// TODO race risk here?
	if r.Difficulty != Custom && (s.recordsBestSolo[r.Difficulty] == nil || r.Time < s.recordsBestSolo[r.Difficulty].Time) {
		s.recordsBestSolo[r.Difficulty] = r
		rs.Best = map[MinesweeperDifficulty]*Record{r.Difficulty: r}
	}
	rs.Partial = true

	log.Printf("<%v> Game submitted %v:%v @ %v", c.Username, m.Mode, m.Difficulty, m.Time)
	s.Broadcast(&Message{RecordSync: rs})
}

func (s *Server) generateRecordsSync() (rs *RecordSyncMessage) {
	rs = &RecordSyncMessage{Best: s.recordsBestSolo}
	records := s.recordsLatest.Prev()
	if records.Value != nil {
		rs.Latest = append(rs.Latest, records.Value.(*Record))
	}
	for p := records.Prev(); p != records; p = p.Prev() {
		if p.Value != nil {
			rs.Latest = append(rs.Latest, p.Value.(*Record))
		}
	}
	return
}
