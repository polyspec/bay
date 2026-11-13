package stack

import "testing"

func TestTheLoadIsSampled(t *testing.T) {
	var s StatsSampler
	if _, err := s.Sample(); err != nil {
		t.Fatal(err)
	}
	st, err := s.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if st.MemTotal == 0 || st.MemUsed == 0 || st.MemUsed > st.MemTotal {
		t.Errorf("memory %d of %d", st.MemUsed, st.MemTotal)
	}
	if st.CPU < 0 || st.CPU > 100 {
		t.Errorf("cpu %.1f", st.CPU)
	}
	if st.SwapUsed > st.SwapTotal {
		t.Errorf("swap %d of %d", st.SwapUsed, st.SwapTotal)
	}
	t.Logf("%+v", st)
}

func TestTheBusyShareIsTheDifference(t *testing.T) {
	var s StatsSampler
	if got := s.shareSince(cpuTimes{busy: 10, total: 100}); got != 0 {
		t.Errorf("first reading %v, want 0", got)
	}
	if got := s.shareSince(cpuTimes{busy: 60, total: 200}); got != 50 {
		t.Errorf("share %v, want 50", got)
	}
}
