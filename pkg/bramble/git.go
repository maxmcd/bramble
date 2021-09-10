package bramble

import (
	"encoding/json"

	build "github.com/maxmcd/bramble/pkg/bramblebuild"
	project "github.com/maxmcd/bramble/pkg/brambleproject"
)

func getGit() (drv build.Derivation, err error) {
	execOutputJSON := `{"Output":{"mfmcau5igf7rbmkkftrkmt7y22vbyavs":{"Args":["-c",""],"Builder":"{{ dsvr3dv6wjmcwcynbwb2fkd55rmukci5:out }}/bin/sh","Dependencies":[{"Hash":"23kutq6mumi6pnb4dbkt44vkgca73yiv","Output":"out"},{"Hash":"dsvr3dv6wjmcwcynbwb2fkd55rmukci5","Output":"out"},{"Hash":"gwnymix2vvo3mnh2pqfupvbflgcmzqdk","Output":"out"}],"Env":{"GIT_EXEC_PATH":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}/libexec/git-core","GIT_SSL_CAINFO":"{{ 23kutq6mumi6pnb4dbkt44vkgca73yiv:out }}","PATH":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}/bin","git":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}"},"Name":"git_fetcher","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}}},"AllDerivations":{"23kutq6mumi6pnb4dbkt44vkgca73yiv":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"https://brmbl.s3.amazonaws.com/ca-certificates.crt"},"Name":"ca-certificates.crt","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"2oscpeuv4wsscndhcbxero2sabmdbzn4":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"https://brmbl.s3.amazonaws.com/busybox-x86_64.tar.gz"},"Name":"busybox-x86_64.tar.gz","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"dsvr3dv6wjmcwcynbwb2fkd55rmukci5":{"Args":["sh","-c","\n    set -e\n    $busybox_download/busybox-x86_64 mkdir $out/bin\n    $busybox_download/busybox-x86_64 cp $busybox_download/busybox-x86_64 $out/bin/busybox\n    cd $out/bin\n    for command in $(./busybox --list); do\n        ./busybox ln -s busybox $command\n    done\n    "],"Builder":"{{ 2oscpeuv4wsscndhcbxero2sabmdbzn4:out }}/busybox-x86_64","Dependencies":[{"Hash":"2oscpeuv4wsscndhcbxero2sabmdbzn4","Output":"out"}],"Env":{"busybox_download":"{{ 2oscpeuv4wsscndhcbxero2sabmdbzn4:out }}"},"Name":"busybox","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"gwnymix2vvo3mnh2pqfupvbflgcmzqdk":{"Args":["-c","\n            set -ex\n            cp -r $src/usr/* $out\n            mkdir test\n            cd test\n            $out/bin/git --version\n            "],"Builder":"{{ dsvr3dv6wjmcwcynbwb2fkd55rmukci5:out }}/bin/sh","Dependencies":[{"Hash":"dsvr3dv6wjmcwcynbwb2fkd55rmukci5","Output":"out"},{"Hash":"wcunj6ugkakaqrejz77jbfoojqxgwzou","Output":"out"}],"Env":{"PATH":"{{ dsvr3dv6wjmcwcynbwb2fkd55rmukci5:out }}/bin","src":"{{ wcunj6ugkakaqrejz77jbfoojqxgwzou:out }}"},"Name":"git","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"mfmcau5igf7rbmkkftrkmt7y22vbyavs":{"Args":["-c",""],"Builder":"{{ dsvr3dv6wjmcwcynbwb2fkd55rmukci5:out }}/bin/sh","Dependencies":[{"Hash":"23kutq6mumi6pnb4dbkt44vkgca73yiv","Output":"out"},{"Hash":"dsvr3dv6wjmcwcynbwb2fkd55rmukci5","Output":"out"},{"Hash":"gwnymix2vvo3mnh2pqfupvbflgcmzqdk","Output":"out"}],"Env":{"GIT_EXEC_PATH":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}/libexec/git-core","GIT_SSL_CAINFO":"{{ 23kutq6mumi6pnb4dbkt44vkgca73yiv:out }}","PATH":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}/bin","git":"{{ gwnymix2vvo3mnh2pqfupvbflgcmzqdk:out }}"},"Name":"git_fetcher","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}},"wcunj6ugkakaqrejz77jbfoojqxgwzou":{"Args":null,"Builder":"fetch_url","Dependencies":null,"Env":{"url":"http://s.minos.io/archive/bifrost/x86_64/git-2.10.2-1.tar.gz"},"Name":"git-2.10.2-1.tar.gz","Outputs":["out"],"Platform":"","Sources":{"Files":null,"Location":""}}}}`

	var output project.ExecModuleOutput

	if err = json.Unmarshal([]byte(execOutputJSON), &output); err != nil {
		return
	}
	drvs, err := runBuildFromOutput(output)
	if err != nil {
		return
	}

	return drvs[0], nil
}
