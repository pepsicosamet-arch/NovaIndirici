plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

// Sürüm, depodaki dist/latest.json ile aynı tutulur (iş akışı -PnovaVersion verir).
val novaVersion: String = (project.findProperty("novaVersion") as String?) ?: "1.5.0"
fun versionCodeOf(v: String): Int {
    val p = v.split(".").map { it.toIntOrNull() ?: 0 }
    return p.getOrElse(0) { 0 } * 10000 + p.getOrElse(1) { 0 } * 100 + p.getOrElse(2) { 0 }
}

android {
    namespace = "com.sametsahin.novaindirici"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.sametsahin.novaindirici"
        minSdk = 24
        targetSdk = 34
        versionName = novaVersion
        versionCode = versionCodeOf(novaVersion)
    }

    signingConfigs {
        create("release") {
            storeFile = file("nova-release.jks")
            storePassword = "novaindirici"
            keyAlias = "nova"
            keyPassword = "novaindirici"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("release")
        }
    }

    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "armeabi-v7a")
            isUniversalApk = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
    buildFeatures {
        compose = true
        buildConfig = true
    }
    packaging {
        jniLibs { useLegacyPackaging = true }
    }
}

dependencies {
    implementation(platform("androidx.compose:compose-bom:2024.09.03"))
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.foundation:foundation")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("androidx.activity:activity-compose:1.9.2")
    implementation("androidx.lifecycle:lifecycle-viewmodel-compose:2.8.6")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.6")
    implementation("androidx.core:core-ktx:1.13.1")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.8.1")
    implementation("io.coil-kt:coil-compose:2.7.0")
    implementation("io.github.junkfood02.youtubedl-android:library:0.17.2")
    implementation("io.github.junkfood02.youtubedl-android:ffmpeg:0.17.2")
}
