```python
cmd("cat file.txt").pipe("sort").pipe("unique").out
cmd("cat file.txt").pipe("sort").pipe("unique").stdout
cmd("cat file.txt").pipe("sort").pipe("unique").stderr

watch = cmd("watch date")
watch.async()

cmd("curl upload").stdin(watch)
// or cmd("curl upload").stdin(watch.stdout)

watch.cancel()

```

this is how you write scripts

you build these things that can do whatever hou want, they xan run in the background, we give sleep and cron primitives that are at the language level so that we can sleep the task when it's incomplete. dependencies can build as the script starts running. we can build what we need first and start executing when we had to

leaves a great possibility for an online editor that is building as you wrote. autocomplete on the code and on the individual scripts themselves.

when we build something it gets a product page with the output of running all commands in the bin folder. We could also allow browsing text files and looking at things, various outputs and such.

this is a free service on top of your github repo similar to the go doc

---

We sandbox by running the bramble script as root and then dipping down to either running as a user or running within docker for the build commands. Keep in mind that we want remote build so this separation should be easy.


## More examples

Server that runs a cron:

```python
load("github.com/sleepokay/nosleepuntilcoachella", "checker")
load("std/runnner", "cron")


def check_for_tickets():
    output = cmd(checker + "/bin/checker").out
    if "available" in output:
        cmd("send email to max@")


cron(every="4h", check_for_tickets)
```

Could also add service/server primitives, databases, etc...
```python
pg = cmd("postgres", environment={"DB":"OKAYSLEEP"}).async())
server(pg)

servers = server(cmd("flask", environment={"DB_URL": server.hostname + ":" + pg.port}).async(), count=2)
```

More scripting examples:


```python
def run_program():
    cmd("redis").async()

    cmd("flask").print()

    cmd("cat foo.txt").pipe("sort").pipe("unique")

    watch = cmd("watch date")
    watch.async()

    cmd("curl upload").stdin(watch)
```
