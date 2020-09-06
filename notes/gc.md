- derivations are left on disk
- derivations can be fetched by hash and name
- outputs can be fetched, this is safe because the derivation contains the system name

If we pack something into a docker container we are limited to just during a derivation into a docker container.

Steps for gc:

 - take all of our derivations that we have tagged as output derivations
 - enumerate their output dependecies, check which output dependecies belong to which input derivations
 - all derivation files and their sources get to stay, delete all outputs we don't depend on
 - delete all recently downloaded files?


----

- keep derivation and sources on the filesystem, that way you can always rebuild the outputs
- remove all derivations that link to nothing
