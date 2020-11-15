
Sessions and cmd calls are becoming more linked. Passing a session around had some nice benefits.

So maybe we just make `cmd` a call off of session, make the link explicit.

We also currently load a working directory. Where should that be? In the current directory?

If we're calling a module we would expect to working in the context of our current directory. This is annoying though because then our current location matters when calling various functions.

Take for example:

Which files should this delete?
```
bramble run ./tasks:remove-files-in-wd
cd ./tasks && bramble run remove-files-in-wd
```

It is tempting to make the working directory the location of the file, but then we run into issue with external modules.

It is tempting to make is the directory that bramble is running in but then we run into inconsistencies depending on where we run the command from.

So maybe we just have it use the module root? That would provided a consistent view to builds.

It is unintuitive though. Maybe more intuitive that the current working directory is the directory where code is being run. For external packages you just use the current directory. Ah, you do need that to take arbitrary action on the current directory with an external lib. That is still inconsistent. We run into the makefile problem yet again.

I say we just leave it at "the working directory where the file is run" and provide access to the module root on the os module so that people can build their own resilient build commands.
