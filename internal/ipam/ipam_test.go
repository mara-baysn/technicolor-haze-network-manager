package ipam

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_AddCIDR_IPv4(t *testing.T) {
	p := NewPool()

	// /30 has 4 addresses, 2 usable (exclude network + broadcast)
	count, err := p.AddCIDR("203.0.113.0/30")
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	avail, alloc := p.Stats()
	assert.Equal(t, 2, avail)
	assert.Equal(t, 0, alloc)
}

func TestPool_AddCIDR_IPv4_Larger(t *testing.T) {
	p := NewPool()

	// /28 has 16 addresses, 14 usable
	count, err := p.AddCIDR("198.51.100.0/28")
	require.NoError(t, err)
	assert.Equal(t, 14, count)
}

func TestPool_AddCIDR_Invalid(t *testing.T) {
	p := NewPool()

	_, err := p.AddCIDR("invalid")
	assert.ErrorIs(t, err, ErrInvalidCIDR)
}

func TestPool_AddIP(t *testing.T) {
	p := NewPool()

	require.NoError(t, p.AddIP("203.0.113.50"))
	require.NoError(t, p.AddIP("2001:db8::1"))

	avail, _ := p.Stats()
	assert.Equal(t, 2, avail)

	// Invalid
	err := p.AddIP("not-an-ip")
	assert.ErrorIs(t, err, ErrInvalidCIDR)
}

func TestPool_Allocate(t *testing.T) {
	p := NewPool()
	require.NoError(t, p.AddIP("203.0.113.10"))
	require.NoError(t, p.AddIP("203.0.113.11"))

	// Allocate
	alloc, err := p.Allocate("tenant-1", IPv4)
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", alloc.TenantID)
	assert.Equal(t, IPv4, alloc.Version)
	assert.False(t, alloc.AllocatedAt.IsZero())

	// Second allocation
	alloc2, err := p.Allocate("tenant-2", IPv4)
	require.NoError(t, err)
	assert.NotEqual(t, alloc.IP, alloc2.IP)

	// No more available
	_, err = p.Allocate("tenant-3", IPv4)
	assert.ErrorIs(t, err, ErrNoAvailableIPs)

	// Check stats
	avail, allocated := p.Stats()
	assert.Equal(t, 0, avail)
	assert.Equal(t, 2, allocated)
}

func TestPool_AllocatePreferVersion(t *testing.T) {
	p := NewPool()
	require.NoError(t, p.AddIP("203.0.113.10"))
	require.NoError(t, p.AddIP("2001:db8::1"))

	// Prefer IPv6
	alloc, err := p.Allocate("tenant-1", IPv6)
	require.NoError(t, err)
	assert.Equal(t, "2001:db8::1", alloc.IP)
	assert.Equal(t, IPv6, alloc.Version)

	// Now only IPv4 available
	alloc2, err := p.Allocate("tenant-2", IPv6)
	require.NoError(t, err)
	assert.Equal(t, IPv4, alloc2.Version)
}

func TestPool_Release(t *testing.T) {
	p := NewPool()
	require.NoError(t, p.AddIP("203.0.113.10"))

	alloc, err := p.Allocate("tenant-1", IPv4)
	require.NoError(t, err)

	// Release
	require.NoError(t, p.Release(alloc.IP))

	avail, allocated := p.Stats()
	assert.Equal(t, 1, avail)
	assert.Equal(t, 0, allocated)

	// Can allocate again
	alloc2, err := p.Allocate("tenant-2", IPv4)
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.10", alloc2.IP)

	// Release non-allocated
	err = p.Release("1.2.3.4")
	assert.ErrorIs(t, err, ErrIPNotAllocated)
}

func TestPool_GetAllocation(t *testing.T) {
	p := NewPool()
	require.NoError(t, p.AddIP("203.0.113.10"))

	_, err := p.Allocate("tenant-1", IPv4)
	require.NoError(t, err)

	alloc, err := p.GetAllocation("203.0.113.10")
	require.NoError(t, err)
	assert.Equal(t, "tenant-1", alloc.TenantID)

	_, err = p.GetAllocation("1.2.3.4")
	assert.ErrorIs(t, err, ErrIPNotAllocated)
}

func TestPool_ListAllocations(t *testing.T) {
	p := NewPool()
	require.NoError(t, p.AddIP("203.0.113.10"))
	require.NoError(t, p.AddIP("203.0.113.11"))
	require.NoError(t, p.AddIP("203.0.113.12"))

	_, err := p.Allocate("tenant-1", IPv4)
	require.NoError(t, err)
	_, err = p.Allocate("tenant-1", IPv4)
	require.NoError(t, err)
	_, err = p.Allocate("tenant-2", IPv4)
	require.NoError(t, err)

	// All
	all := p.ListAllocations("")
	assert.Len(t, all, 3)

	// Filtered
	t1 := p.ListAllocations("tenant-1")
	assert.Len(t, t1, 2)

	t2 := p.ListAllocations("tenant-2")
	assert.Len(t, t2, 1)
}

func TestPool_ConcurrentAccess(t *testing.T) {
	p := NewPool()

	// Add a bunch of IPs
	_, err := p.AddCIDR("198.51.100.0/24")
	require.NoError(t, err)

	// Concurrent allocations
	done := make(chan *Allocation, 100)
	for i := 0; i < 100; i++ {
		go func(id int) {
			alloc, _ := p.Allocate("tenant-1", IPv4)
			done <- alloc
		}(i)
	}

	allocated := make(map[string]bool)
	for i := 0; i < 100; i++ {
		alloc := <-done
		if alloc != nil {
			assert.False(t, allocated[alloc.IP], "duplicate allocation: %s", alloc.IP)
			allocated[alloc.IP] = true
		}
	}
	assert.Equal(t, 100, len(allocated))
}
