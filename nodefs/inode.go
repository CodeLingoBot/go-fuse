// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"log"
	"sort"
	"strings"
	"sync"
	"unsafe"

	"github.com/hanwen/go-fuse/fuse"
)

var _ = log.Println

type parentData struct {
	name   string
	parent *Inode
}

// Inode is a node in VFS tree.  Inodes are one-to-one mapped to Node
// instances, which is the extension interface for file systems.  One
// can create fully-formed trees of Inodes ahead of time by creating
// "persistent" Inodes.
type Inode struct {
	// The filetype bits from the mode.
	mode     uint32
	opaqueID uint64
	node     Node
	bridge   *rawBridge

	// Following data is mutable.

	// Protected by bridge.mu
	lookupCount uint64
	nodeID      uint64

	// mu protects the following mutable fields. When locking
	// multiple Inodes, locks must be acquired using
	// lockNodes/unlockNodes
	mu sync.Mutex

	// incremented every time the 'children' or 'parents' field is changed.
	changeCounter uint32
	children      map[string]*Inode
	parents       map[parentData]struct{}
}

func sortNodes(ns []*Inode) {
	sort.Slice(ns, func(i, j int) bool {
		return uintptr(unsafe.Pointer(ns[i])) < uintptr(unsafe.Pointer(ns[j]))
	})
}

func lockNodes(ns ...*Inode) {
	sortNodes(ns)
	for _, n := range ns {
		n.mu.Lock()
	}
}

func unlockNodes(ns ...*Inode) {
	sortNodes(ns)
	for _, n := range ns {
		n.mu.Unlock()
	}
}

// Forgotten returns true if the kernel holds no references to this
// inode.  This can be used for background cleanup tasks, since the
// kernel has no way of reviving forgotten nodes by its own
// initiative.
func (n *Inode) Forgotten() bool {
	n.bridge.mu.Lock()
	defer n.bridge.mu.Unlock()
	return n.lookupCount == 0
}

// Node returns the Node object implementing the file system operations.
func (n *Inode) Node() Node {
	return n.node
}

// Path returns a path string to the inode relative to the root.
func (n *Inode) Path(root *Inode) string {
	var segments []string
	p := n
	for p != nil && p != root {
		var pd parentData

		// We don't try to take all locks at the same time, because
		// the caller won't use the "path" string under lock anyway.
		p.mu.Lock()
		for pd = range p.parents {
			break
		}
		p.mu.Unlock()
		if pd.parent == nil {
			break
		}

		segments = append(segments, pd.name)
		p = pd.parent
	}

	if p == nil {
		// NOSUBMIT - should replace rather than append?
		segments = append(segments, ".deleted")
	}

	i := 0
	j := len(segments) - 1

	for i < j {
		segments[i], segments[j] = segments[j], segments[i]
		i++
		j--
	}

	path := strings.Join(segments, "/")
	return path
}

// Finds a child with the given name and filetype.  Returns nil if not
// found.
func (n *Inode) FindChildByMode(name string, mode uint32) *Inode {
	mode ^= 07777

	n.mu.Lock()
	defer n.mu.Unlock()

	ch := n.children[name]

	if ch != nil && ch.mode == mode {
		return ch
	}

	return nil
}

// Finds a child with the given name and ID. Returns nil if not found.
func (n *Inode) FindChildByOpaqueID(name string, opaqueID uint64) *Inode {
	n.mu.Lock()
	defer n.mu.Unlock()

	ch := n.children[name]

	if ch != nil && ch.opaqueID == opaqueID {
		return ch
	}

	return nil
}

func (n *Inode) addLookup(name string, child *Inode) {
	child.lookupCount++
	child.parents[parentData{name, n}] = struct{}{}
	n.children[name] = child
	child.changeCounter++
	n.changeCounter++
}

func (n *Inode) clearParents() {
	for {
		lockme := []*Inode{n}
		n.mu.Lock()
		ts := n.changeCounter
		for p := range n.parents {
			lockme = append(lockme, p.parent)
		}
		n.mu.Unlock()

		lockNodes(lockme...)
		success := false
		if ts == n.changeCounter {
			for p := range n.parents {
				delete(p.parent.children, p.name)
				p.parent.changeCounter++
			}
			n.parents = map[parentData]struct{}{}
			n.changeCounter++
			success = true
		}
		unlockNodes(lockme...)

		if success {
			return
		}
	}
}

func (n *Inode) clearChildren() {
	if n.mode != fuse.S_IFDIR {
		return
	}

	var lockme []*Inode
	for {
		lockme = append(lockme[:0], n)

		n.mu.Lock()
		ts := n.changeCounter
		for _, ch := range n.children {
			lockme = append(lockme, ch)
		}
		n.mu.Unlock()

		lockNodes(lockme...)
		success := false
		if ts == n.changeCounter {
			for nm, ch := range n.children {
				delete(ch.parents, parentData{nm, n})
				ch.changeCounter++
			}
			n.children = map[string]*Inode{}
			n.changeCounter++
			success = true
		}
		unlockNodes(lockme...)

		if success {
			break
		}
	}

	for _, ch := range lockme {
		if ch != n {
			ch.clearChildren()
		}
	}
}

// NewPersistentInode returns an Inode with a LookupCount == 1, ie. the
// node will only get garbage collected if the kernel issues a forget
// on any of its parents.
func (n *Inode) NewPersistentInode(node Node, mode uint32, opaque uint64) *Inode {
	ch := n.NewInode(node, mode, opaque)
	ch.lookupCount++
	return ch
}

// NewInode returns an inode for the given Node. The mode should be
// standard mode argument (eg. S_IFDIR). The opaqueID argument can be
// used to signal changes in the tree structure during lookup (see
// FindChildByOpaqueID). For a loopback file system, the inode number
// of the underlying file is a good candidate.
func (n *Inode) NewInode(node Node, mode uint32, opaqueID uint64) *Inode {
	ch := &Inode{
		mode:    mode ^ 07777,
		node:    node,
		bridge:  n.bridge,
		parents: make(map[parentData]struct{}),
	}
	if mode&fuse.S_IFDIR != 0 {
		ch.children = make(map[string]*Inode)
	}
	node.setInode(ch)
	return ch
}
