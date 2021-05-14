var gulp = require("gulp");
var strip = require("gulp-strip-comments");
var cleanCSS = require("gulp-clean-css");
var concat = require("gulp-concat");

gulp.task("compress-css", function () {
    return gulp
        .src([
            "node_modules/leaflet/dist/leaflet.css",
            "node_modules/leaflet-zoombox/L.Control.ZoomBox.css",
            "node_modules/leaflet-basemaps/L.Control.Basemaps.css",
            "node_modules/leaflet-range/L.Control.Range.css",
            "node_modules/leaflet-base64-legend/L.Control.Base64Legend.css",
        ])
        .pipe(cleanCSS())
        .pipe(concat("core.min.css"))
        .pipe(gulp.dest("dist"));
});

gulp.task("concat-js", function () {
    return gulp
        .src([
            "node_modules/d3-collection/dist/d3-collection.min.js",
            "node_modules/d3-fetch/dist/d3-fetch.min.js",
            "node_modules/leaflet/dist/leaflet.js",
            "node_modules/leaflet-zoombox/L.Control.ZoomBox.min.js",
            "node_modules/leaflet-basemaps/L.Control.Basemaps-min.js",
            "node_modules/leaflet-range/L.Control.Range-min.js",
            "node_modules/leaflet-base64-legend/L.Control.Base64Legend-min.js",
            "node_modules/leaflet-utfgrid/L.UTFGrid-min.js",
        ])
        .pipe(strip())
        .pipe(concat("core.min.js"))
        .pipe(gulp.dest("dist"));
});

gulp.task("build", gulp.series("compress-css", "concat-js"));
