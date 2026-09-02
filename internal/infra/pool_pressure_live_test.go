//go:build integration && postgres

// Measures what the concurrent sign-in read costs a BOUNDED connection pool,
// which is the one thing the read redesign never tested: every number behind it
// came from one sign-in at a time.
//
// ResolveSignIn issues its four statements concurrently, so one sign-in holds
// four pool connections at once where the shape it replaced held one. The
// question is not whether that is faster in isolation — it is, and by a round
// trip — but whether the peak it creates degrades under load, and whether the
// serial shape would degrade less.
//
// So this file runs BOTH shapes against the same rows, the same pool and the
// same concurrency, and reports them side by side. Nothing here asserts a
// threshold: a latency number is a property of the machine it ran on, and a test
// that failed on a slow laptop would be deleted rather than read. It fails only
// on an ERROR — a refused acquire, a deadline, a query that broke — because
// those are properties of the shape and not of the hardware.
//
// It needs the loadtest fixture (tenant 11111111-1111-1111-1111-111111111111);
// it skips when that tenant is absent rather than seeding 17k rows itself.

package infra

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ClaudioSchirmer/authcore/internal/domain/vos"
	"github.com/ClaudioSchirmer/authcore/internal/infra/schemas"
	"github.com/ClaudioSchirmer/omnicore/infra/db/core"
	"github.com/ClaudioSchirmer/omnicore/infra/db/criteria"
	"github.com/ClaudioSchirmer/omnicore/infra/db/engine/postgres"
)

const loadTestEmail = "lt_user_1@loadtest.local"

// resolveSignInSerially is the shape ResolveSignIn replaced, kept HERE and not in
// production: the same four statements, one after another, so the comparison is
// between two orderings of identical work rather than between two queries.
func (r *AuthenticationReader) resolveSignInSerially(
	ctx context.Context, account *schemas.SignInAccount,
) (SignInBundle, error) {
	directRows, err := r.directGrants.FindAll(ctx,
		criteria.Where(criteria.Eq("ParentID", account.ID)))
	if err != nil {
		return SignInBundle{}, err
	}
	inheritedRows, err := r.inheritedGrants.FindAll(ctx,
		criteria.Where(criteria.Eq("ParentID", account.ID)))
	if err != nil {
		return SignInBundle{}, err
	}
	valueRows, err := r.claimValues.FindAll(ctx,
		criteria.Where(criteria.Eq("ParentID", account.ID)))
	if err != nil {
		return SignInBundle{}, err
	}
	definitions, err := r.definitions.FindAll(ctx, criteria.Where(criteria.And(
		criteria.Eq("TenantID", account.TenantID),
		criteria.In("AppliesTo",
			vos.ClaimAppliesToUser.Value(),
			vos.ClaimAppliesToBoth.Value()),
	)))
	if err != nil {
		return SignInBundle{}, err
	}
	return assemble(directRows, inheritedRows, valueRows, definitions), nil
}

// poolReader opens an engine whose pool is bounded to exactly maxConns, which is
// the whole point: the default is max(4, NumCPU) and the machine this runs on
// would otherwise hide the pressure it is here to measure.
func poolReader(t *testing.T, maxConns int) (*AuthenticationReader, *postgres.Postgres) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://omnicore:omnicore@localhost:5432/authcore_db?sslmode=disable"
	}
	eng, err := postgres.NewPostgres(context.Background(), dsn,
		postgres.WithPool(core.PoolConfig{MaxOpenConns: maxConns}))
	if err != nil {
		t.Skipf("no dev bench reachable: %v", err)
	}
	eng.SetClock(core.ClockDB)
	t.Cleanup(eng.Close)
	return NewAuthenticationReader(eng), eng
}

type sample struct {
	latencies []time.Duration
	errs      int
	elapsed   time.Duration
}

func (s sample) at(q float64) time.Duration {
	if len(s.latencies) == 0 {
		return 0
	}
	i := int(float64(len(s.latencies)-1) * q)
	return s.latencies[i]
}

func (s sample) rps() float64 {
	if s.elapsed == 0 {
		return 0
	}
	return float64(len(s.latencies)) / s.elapsed.Seconds()
}

// drive runs `iterations` resolutions spread over `workers` goroutines and
// returns the sorted latencies. The account is loaded ONCE and shared: the
// account read is one statement outside the fan-out, and including it would
// measure a fifth connection this comparison is not about.
func drive(
	t *testing.T,
	reader *AuthenticationReader,
	account *schemas.SignInAccount,
	workers, iterations int,
	resolve func(context.Context, *schemas.SignInAccount) (SignInBundle, error),
) sample {
	t.Helper()
	var (
		mu   sync.Mutex
		out  sample
		wg   sync.WaitGroup
		each = iterations / workers
	)
	// A deadline in the same order as the service's own request timeout, so a
	// blocked acquire surfaces as an error here the way it would there.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]time.Duration, 0, each)
			localErrs := 0
			for i := 0; i < each; i++ {
				at := time.Now()
				if _, err := resolve(ctx, account); err != nil {
					localErrs++
					continue
				}
				local = append(local, time.Since(at))
			}
			mu.Lock()
			out.latencies = append(out.latencies, local...)
			out.errs += localErrs
			mu.Unlock()
		}()
	}
	wg.Wait()
	out.elapsed = time.Since(start)
	sort.Slice(out.latencies, func(i, j int) bool { return out.latencies[i] < out.latencies[j] })
	return out
}

func loadTestAccount(t *testing.T, reader *AuthenticationReader) *schemas.SignInAccount {
	t.Helper()
	account, err := reader.LoadAccountByEmail(context.Background(), loadTestEmail)
	if err != nil || account == nil {
		t.Skipf("loadtest fixture absent (%s): %v", loadTestEmail, err)
	}
	return account
}

// TestPoolPressure_ConcurrentVsSerial is the measurement the backlog entry asks
// for: N concurrent sign-in reads against a pool of a production-plausible size,
// in both shapes.
func TestPoolPressure_ConcurrentVsSerial(t *testing.T) {
	const iterations = 400

	for _, pool := range []int{2, 4, 8, 16} {
		reader, _ := poolReader(t, pool)
		account := loadTestAccount(t, reader)

		// One warm pass so connection establishment is not charged to the first
		// measured request.
		drive(t, reader, account, 4, 40, reader.ResolveSignIn)

		for _, workers := range []int{1, 4, 16, 64} {
			conc := drive(t, reader, account, workers, iterations, reader.ResolveSignIn)
			serial := drive(t, reader, account, workers, iterations, reader.resolveSignInSerially)

			fmt.Printf("pool=%-3d workers=%-4d | concurrent p50=%-9s p95=%-9s p99=%-9s %7.0f rps errs=%d"+
				" | serial p50=%-9s p95=%-9s p99=%-9s %7.0f rps errs=%d\n",
				pool, workers,
				conc.at(0.50).Round(time.Microsecond), conc.at(0.95).Round(time.Microsecond),
				conc.at(0.99).Round(time.Microsecond), conc.rps(), conc.errs,
				serial.at(0.50).Round(time.Microsecond), serial.at(0.95).Round(time.Microsecond),
				serial.at(0.99).Round(time.Microsecond), serial.rps(), serial.errs)

			if conc.errs > 0 || serial.errs > 0 {
				t.Errorf("pool=%d workers=%d: resolutions failed (concurrent=%d serial=%d) —"+
					" a bounded pool must queue, never refuse", pool, workers, conc.errs, serial.errs)
			}
		}
	}
}

// TestPoolPressure_ExhaustionIsQueueingNotDeadlock pins the property the fan-out
// could plausibly break: four goroutines per request, against a pool of ONE.
//
// It cannot deadlock — no goroutine holds a connection while waiting for another
// — and that is a claim about the code, so it is worth a row that proves it
// rather than a paragraph asserting it. A pool of one is the smallest bound that
// can express the failure: if the fan-out were reentrant, this would hang until
// the deadline instead of returning.
func TestPoolPressure_ExhaustionIsQueueingNotDeadlock(t *testing.T) {
	reader, _ := poolReader(t, 1)
	account := loadTestAccount(t, reader)

	done := make(chan sample, 1)
	go func() { done <- drive(t, reader, account, 32, 320, reader.ResolveSignIn) }()

	select {
	case got := <-done:
		if got.errs > 0 {
			t.Fatalf("pool=1: %d resolutions failed; a starved pool must queue, not refuse", got.errs)
		}
		fmt.Printf("pool=1   workers=32   | 320 resolutions in %s, p99=%s — queued, never deadlocked\n",
			got.elapsed.Round(time.Millisecond), got.at(0.99).Round(time.Microsecond))
	case <-time.After(60 * time.Second):
		t.Fatal("pool=1: the fan-out did not finish in 60s — it is holding a connection while waiting for one")
	}
}

// TestPoolPressure_AgainstTheCostOfTheCredential puts the read next to the thing
// that shares its request, because the pool question is only interesting
// relative to it.
//
// A sign-in verifies an Argon2id hash (m=19MiB, t=2) BEFORE it reads anything.
// That is deliberately expensive and it is CPU and memory, not a connection —
// so if it dominates, the endpoint saturates its cores long before the read
// saturates the pool, and the fan-out's four connections are being held during
// a window nothing else is competing for.
func TestPoolPressure_AgainstTheCostOfTheCredential(t *testing.T) {
	reader, _ := poolReader(t, 4)
	account := loadTestAccount(t, reader)

	hasher := NewArgon2idHasher()
	encoded := hasher.Hash("a-password-of-realistic-length")

	const rounds = 20
	at := time.Now()
	for i := 0; i < rounds; i++ {
		if !hasher.Matches("a-password-of-realistic-length", encoded) {
			t.Fatal("the hasher rejected its own hash")
		}
	}
	perVerify := time.Since(at) / rounds

	read := drive(t, reader, account, 1, 100, reader.ResolveSignIn)
	fmt.Printf("credential verify = %s per sign-in | the four-statement read = %s"+
		" | the read is %.1f%% of the two\n",
		perVerify.Round(time.Microsecond), read.at(0.50).Round(time.Microsecond),
		100*float64(read.at(0.50))/float64(perVerify+read.at(0.50)))
}

// TestPoolPressure_TheNeighbourIsWhatPaysForIt asks the question the sign-in
// path cannot ask about itself.
//
// A fan-out that quadruples one endpoint's peak does not hurt that endpoint
// much — it queues, and the numbers above show it queues linearly. What it can
// hurt is EVERY OTHER ROUTE, which draws from the same bounded pool and has no
// idea a sign-in burst is happening. So the measurement here is a neighbour: one
// ordinary single-statement read, timed while a realistic sign-in load runs
// beside it, under both shapes.
//
// The sign-in load is realistic on purpose — it verifies an Argon2id hash before
// each read, the way the endpoint does. Driving the read alone would model a
// burst no credential check could ever sustain, and would answer a question
// nobody has.
func TestPoolPressure_TheNeighbourIsWhatPaysForIt(t *testing.T) {
	for _, pool := range []int{4, 8, 16, 32} {
		neighbourUnderLoad(t, pool)
	}
}

func neighbourUnderLoad(t *testing.T, pool int) {
	reader, _ := poolReader(t, pool)
	account := loadTestAccount(t, reader)
	hasher := NewArgon2idHasher()
	encoded := hasher.Hash("a-password-of-realistic-length")

	measure := func(
		label string,
		resolve func(context.Context, *schemas.SignInAccount) (SignInBundle, error),
	) {
		ctx, cancel := context.WithCancel(context.Background())
		var load sync.WaitGroup

		// Sixteen concurrent sign-ins, each paying the credential cost first.
		for w := 0; w < 16; w++ {
			load.Add(1)
			go func() {
				defer load.Done()
				for ctx.Err() == nil {
					hasher.Matches("a-password-of-realistic-length", encoded)
					if _, err := resolve(ctx, account); err != nil {
						return
					}
				}
			}()
		}

		// The neighbour: one plain account read, the cheapest statement the
		// service runs, sampled while the burst is in flight.
		time.Sleep(200 * time.Millisecond)
		var neighbour []time.Duration
		for i := 0; i < 200; i++ {
			at := time.Now()
			if _, err := reader.LoadAccountByEmail(context.Background(), loadTestEmail); err != nil {
				t.Fatalf("%s: the neighbour failed: %v", label, err)
			}
			neighbour = append(neighbour, time.Since(at))
		}
		cancel()
		load.Wait()

		sort.Slice(neighbour, func(i, j int) bool { return neighbour[i] < neighbour[j] })
		fmt.Printf("pool=%-3d neighbour under %-10s load | p50=%-9s p95=%-9s p99=%-9s max=%s\n", pool, label,
			neighbour[len(neighbour)/2].Round(time.Microsecond),
			neighbour[len(neighbour)*95/100].Round(time.Microsecond),
			neighbour[len(neighbour)*99/100].Round(time.Microsecond),
			neighbour[len(neighbour)-1].Round(time.Microsecond))
	}

	measure("concurrent", reader.ResolveSignIn)
	measure("serial", reader.resolveSignInSerially)

	// And the same neighbour with nothing else running, as the baseline the two
	// rows above are read against.
	var idle []time.Duration
	for i := 0; i < 200; i++ {
		at := time.Now()
		if _, err := reader.LoadAccountByEmail(context.Background(), loadTestEmail); err != nil {
			t.Fatalf("idle neighbour failed: %v", err)
		}
		idle = append(idle, time.Since(at))
	}
	sort.Slice(idle, func(i, j int) bool { return idle[i] < idle[j] })
	fmt.Printf("pool=%-3d neighbour under %-10s load | p50=%-9s p95=%-9s p99=%-9s max=%s\n", pool, "no",
		idle[len(idle)/2].Round(time.Microsecond),
		idle[len(idle)*95/100].Round(time.Microsecond),
		idle[len(idle)*99/100].Round(time.Microsecond),
		idle[len(idle)-1].Round(time.Microsecond))
}

// TestPoolPressure_IsolatingPoolFromCPU separates the two things the neighbour
// test above conflates.
//
// That test runs sixteen Argon2id verifications at once on the same machine as
// the database, so at that width the neighbour is competing for CPU, not for a
// connection — which is why widening the pool did not help it. Here the sign-in
// load is swept from narrow to wide against a FIXED pool: at the narrow end the
// CPU is idle and any gap between the shapes is the pool; at the wide end the
// CPU is saturated and the gap is the machine.
func TestPoolPressure_IsolatingPoolFromCPU(t *testing.T) {
	const pool = 4
	reader, _ := poolReader(t, pool)
	account := loadTestAccount(t, reader)
	hasher := NewArgon2idHasher()
	encoded := hasher.Hash("a-password-of-realistic-length")

	measure := func(
		width int, withCredential bool, label string,
		resolve func(context.Context, *schemas.SignInAccount) (SignInBundle, error),
	) time.Duration {
		ctx, cancel := context.WithCancel(context.Background())
		var load sync.WaitGroup
		for w := 0; w < width; w++ {
			load.Add(1)
			go func() {
				defer load.Done()
				for ctx.Err() == nil {
					if withCredential {
						hasher.Matches("a-password-of-realistic-length", encoded)
					}
					if _, err := resolve(ctx, account); err != nil {
						return
					}
				}
			}()
		}
		time.Sleep(150 * time.Millisecond)
		var neighbour []time.Duration
		for i := 0; i < 150; i++ {
			at := time.Now()
			if _, err := reader.LoadAccountByEmail(context.Background(), loadTestEmail); err != nil {
				t.Fatalf("%s: neighbour failed: %v", label, err)
			}
			neighbour = append(neighbour, time.Since(at))
		}
		cancel()
		load.Wait()
		sort.Slice(neighbour, func(i, j int) bool { return neighbour[i] < neighbour[j] })
		return neighbour[len(neighbour)/2]
	}

	fmt.Printf("%-8s %-12s | %-12s %-12s %s\n", "width", "credential", "concurrent", "serial", "ratio")
	for _, withCredential := range []bool{true, false} {
		for _, width := range []int{1, 2, 4, 8, 16} {
			c := measure(width, withCredential, "concurrent", reader.ResolveSignIn)
			s := measure(width, withCredential, "serial", reader.resolveSignInSerially)
			fmt.Printf("%-8d %-12v | %-12s %-12s %.2fx\n", width, withCredential,
				c.Round(time.Microsecond), s.Round(time.Microsecond),
				float64(c)/float64(s))
		}
	}
}

// TestPoolPressure_WhereTheCeilingActuallyIs puts a rate on the width axis, so
// the point where the neighbour starts paying can be stated in sign-ins per
// second rather than in goroutines.
//
// It measures the FULL cost of a sign-in — the Argon2id verification plus the
// four-statement read — which is what a request actually spends. The credential
// check is CPU and the read is a connection, so the ceiling this reports is the
// machine's, and it is the number the pool has to be judged against.
func TestPoolPressure_WhereTheCeilingActuallyIs(t *testing.T) {
	const pool = 4
	reader, _ := poolReader(t, pool)
	account := loadTestAccount(t, reader)
	hasher := NewArgon2idHasher()
	encoded := hasher.Hash("a-password-of-realistic-length")

	for _, width := range []int{1, 4, 8, 16} {
		var (
			wg    sync.WaitGroup
			mu    sync.Mutex
			count int
		)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		start := time.Now()
		for w := 0; w < width; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				local := 0
				for ctx.Err() == nil {
					hasher.Matches("a-password-of-realistic-length", encoded)
					if _, err := reader.ResolveSignIn(ctx, account); err != nil {
						break
					}
					local++
				}
				mu.Lock()
				count += local
				mu.Unlock()
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)
		cancel()
		fmt.Printf("width=%-3d | %.0f full sign-ins/s (credential + the four-statement read)\n",
			width, float64(count)/elapsed.Seconds())
	}
}
