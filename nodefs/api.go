// Copyright 2019 the Go-FUSE Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package nodefs

import (
	"context"
	"time"

	"github.com/hanwen/go-fuse/fuse"
)

/*
NOSUBMIT: how to structure?

- one interface per method?
- one interface for files (getattr, read/write), one for dirs (lookup, opendir), one shared?
- one giant interface?
- use raw types as args rather than mimicking Golang signatures?

*/
type Node interface {
	// setInode links the Inode to a Node.
	setInode(*Inode)

	// Inode must return a non-nil associated inode structure. The
	// identity of the Inode may not change over the lifetime of
	// the object.
	Inode() *Inode

	// Lookup finds a child Inode. If a new Inode must be created,
	// the inode does not have to be added to the tree.
	Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*Inode, fuse.Status)

	Open(ctx context.Context, flags uint32) (fh File, fuseFlags uint32, code fuse.Status)

	Create(ctx context.Context, name string, flags uint32, mode uint32) (inode *Inode, fh File, fuseFlags uint32, code fuse.Status)

	Read(ctx context.Context, f File, dest []byte, off int64) (fuse.ReadResult, fuse.Status)

	Write(ctx context.Context, f File, data []byte, off int64) (written uint32, code fuse.Status)

	// File locking
	GetLk(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status)
	SetLk(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)
	SetLkw(ctx context.Context, f File, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)

	// Flush is called for close() call on a file descriptor. In
	// case of duplicated descriptor, it may be called more than
	// once for a file.
	Flush(ctx context.Context, f File) fuse.Status

	// This is called to before the file handle is forgotten. This
	// method has no return value, so nothing can synchronizes on
	// the call. Any cleanup that requires specific synchronization or
	// could fail with I/O errors should happen in Flush instead.
	Release(ctx context.Context, f File)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	GetAttr(ctx context.Context, f File, out *fuse.Attr) fuse.Status

	/*
		NOSUBMIT - fold into a setattr method, or expand methods?

		Decoding SetAttr is a bit of a PITA, but if we use fuse
		types as args, we can't take apart SetAttr for the caller
	*/

	Truncate(ctx context.Context, f File, size uint64) fuse.Status
	Chown(ctx context.Context, f File, uid uint32, gid uint32) fuse.Status
	Chmod(ctx context.Context, f File, perms uint32) fuse.Status
	Utimens(ctx context.Context, f File, atime *time.Time, mtime *time.Time) fuse.Status
	Allocate(ctx context.Context, f File, off uint64, size uint64, mode uint32) (code fuse.Status)
}

type File interface {
	Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, fuse.Status)
	Write(ctx context.Context, data []byte, off int64) (written uint32, code fuse.Status)

	// File locking
	GetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32, out *fuse.FileLock) (code fuse.Status)
	SetLk(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)
	SetLkw(ctx context.Context, owner uint64, lk *fuse.FileLock, flags uint32) (code fuse.Status)

	// Flush is called for close() call on a file descriptor. In
	// case of duplicated descriptor, it may be called more than
	// once for a file.
	Flush(ctx context.Context) fuse.Status

	// This is called to before the file handle is forgotten. This
	// method has no return value, so nothing can synchronizes on
	// the call. Any cleanup that requires specific synchronization or
	// could fail with I/O errors should happen in Flush instead.
	Release(ctx context.Context)

	// The methods below may be called on closed files, due to
	// concurrency.  In that case, you should return EBADF.
	// TODO - fold into a setattr method?
	GetAttr(ctx context.Context, out *fuse.Attr) fuse.Status
	Truncate(ctx context.Context, size uint64) fuse.Status
	Chown(ctx context.Context, uid uint32, gid uint32) fuse.Status
	Chmod(ctx context.Context, perms uint32) fuse.Status
	Utimens(ctx context.Context, atime *time.Time, mtime *time.Time) fuse.Status
	Allocate(ctx context.Context, off uint64, size uint64, mode uint32) (code fuse.Status)
}

type Options struct {
	Debug bool

	EntryTimeout    *time.Duration
	AttrTimeout     *time.Duration
	NegativeTimeout *time.Duration
}
