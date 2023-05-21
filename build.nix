{ pkgs, buildGoModule, lib }: buildGoModule rec {
    src = ./.;

    name = "github.com/ayes-web/nyaasi_scraper";
    vendorSha256 = "sha256-Ls0E4VvDu4l9a1n1TO8JL2PKvGE34NVwF9BFA5LwS8g=";

    ldflags = [
        "-s"
        "-w"
    ];
}