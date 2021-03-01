


you can pass an array with each cached derivation at each index. That way you don't need to read files or hash any content, you can just load the derivation from the filesystem. No file copying, no extra reads. (will need a stack for regular execution to track derivation count. also how does this work with parallel execution :( )q
