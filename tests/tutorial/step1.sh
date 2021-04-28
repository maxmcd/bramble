set -e

mkdir $out/bin

# write the file contents
echo "#!$bash
echo 'hello there!'" > $out/bin/say_hi
chmod +x $out/bin/say_hi

# test it
$out/bin/say_hi
