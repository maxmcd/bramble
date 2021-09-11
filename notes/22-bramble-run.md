Bramble run and bramble shell


Bramble run works to run a function, by default it takes no stdin and doesn't take the environment's variables.

Bramble shell runs a shell, takes environment variables from the local environment (maybe even allows RO access to $HOME??), but is not allowed for external deps.


When either of these are called the drv $out/bin is added to the path.

