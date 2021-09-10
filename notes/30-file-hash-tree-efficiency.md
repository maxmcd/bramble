
File copying and hashing is expensive. Could hash each file and assemble in a tree. When an individual file is changed we just hash that file and then re-hash the file hash values. This would make incremental changes less painful. The state of the previous hash calculation would need to sit on disk somewhere so that we don't have to re-calculate those hashes every time.

Wait, we could do this for all file folders, and store the file-maps remotely. That way if you pull a dependency you can run it immediately and pull the files down as you need them. We could do that with https://github.com/google/crfs/blob/master/crfs.go so that we can get fast starting downloads. On linux at least. Would need to have osxfuse installed for it to work on osx.

So, not sure where this should go. These are two thoughts tho, one is about using a tree to hash, the other is about allowing one to retriteve their contents.

We should try out the first one though, would make things fast.

Ah man, this also means you can start building immediately. If you use the fuse layer to serve the files then you can start building before the sources are tarred up and sent out. You can finish building before the build state arrives.

Would need to be able to disable it in the derivation definition.
