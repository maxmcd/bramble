
File copying and hashing is expensive. Could hash each file and assemble in a tree. When an individual file is changed we just hash that file and then re-hash the file hash values. This would make incremental changes less painful. The state of the previous hash calculation would need to sit on disk somewhere so that we don't have to re-calculate those hashes every time.

Wait, we could do this for all file folders, and store the file-maps remotely. That way if you pull a dependency you can run it immediately and pull the files down as you need them. We could do that with https://github.com/google/crfs/blob/master/crfs.go so that we can get fast starting downloads. On linux at least. Would need to have osxfuse installed for it to work on osx.

So, not sure where this should go. These are two thoughts tho, one is about using a tree to hash, the other is about allowing one to retriteve their contents.

We should try out the first one though, would make things fast.

Ah man, this also means you can start building immediately. If you use the fuse layer to serve the files then you can start building before the sources are tarred up and sent out. You can finish building before the build state arrives.

Would need to be able to disable it in the derivation definition.


---------

Thinking about this again. I think it separates into a few things.


One is hashing of sources by hashing the individual files. This is good because then we can quickly hash sources. All we can do is speed up hashing, since we have to still copy every file into the build context when building.

Another is fast startup for remote builds. If we have a remote build that has a fuse filesystem we could prioritize file fetches when `open` is called on a specific file

Another is de-duplication. If we store files as chunks of files then we de-duplicate against chunks. This would be good for pushing up files and skipping common files. Not a huge space-saving though.

So for the "now", how does affect the build cache? build cache would just have derivations, and outputs. Outputs are files, so we could get de-duplication and faster remote starts.

Ok, so stargz or hashed chunks?

---

So, we use the stargz toc entry to track files. Add output metadata to the cache upload. File contents are chunked up into <4mb chunks and the hashes are stored in the toc entry file.
