{ pkgs, default, nix2container}: nix2container.packages.${pkgs.system}.nix2container.buildImage {
    name = "ghcr.io/ayes-web/nyaasi_scraper";
    tag = "latest";

    copyToRoot = pkgs.cacert;

    config = {
        entrypoint = ["${default}/bin/nyaasi_scraper" "--database" "file:/app/database.db?cache=shared&_busy_timeout=1000000"];
    };
}