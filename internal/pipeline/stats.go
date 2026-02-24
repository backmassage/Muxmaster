package pipeline

// RunStats tracks aggregate counters and byte totals across a batch run.
type RunStats struct {
	Total            int
	Current          int
	Encoded          int
	Skipped          int
	Failed           int
	TotalInputBytes  int64
	TotalOutputBytes int64
}

// SpaceSaved returns the aggregate byte difference between inputs and outputs.
// Positive means outputs are smaller; negative means they grew.
func (s *RunStats) SpaceSaved() int64 {
	return s.TotalInputBytes - s.TotalOutputBytes
}
