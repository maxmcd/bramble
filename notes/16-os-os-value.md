case "expand":
case "cd":
case "getenv":
case "setenv":

if we pass the following four methods in the form of a session then we can cache function calls. if these must be passed and the calling function modifies no files we can trivially compute the cache. this is also good because it prevents function calls from easily modifying global state and altering execution expectations.

if we pass a session by value we can build various values onto it. could be passed to cmd(), derivation, etc.., would even be a way to dynamically build derivation inputs. (maybe that's a bad idea). no, if we needed a certain program in the path, that could help, still likely better for us to just support build_inputs.

----

thought about this some more, broke things out into sessions

there still needs to be a solution to the bramble dirty inputs problem, should just use the current option of calculating the derivation and then only erroring on the derivation call if we haven't already seen that function call before. Performance can come later

should we remove environment variables entirely? are they useful in scripts?
shoudl we pass things by value into functions?
