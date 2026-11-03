const {chromium}=require(process.env.CV_PLAYWRIGHT_MODULE || 'playwright');
const assert=require('node:assert/strict');
(async()=>{
 const browser=await chromium.launch({...(process.env.CV_CHROMIUM_PATH ? {executablePath:process.env.CV_CHROMIUM_PATH} : {}),headless:true});
 try {
  for(const [name,width,height] of [['desktop',1280,900],['mobile',390,844]]){
   const context=await browser.newContext({viewport:{width,height},acceptDownloads:true});
   const page=await context.newPage();const errors=[];page.on('pageerror',e=>errors.push(e.message));
   await page.route('**/*',route=>new URL(route.request().url()).origin===process.argv[2]?route.continue():route.abort());
   await context.request.get(process.argv[2]+'/fixture/developer');
   await page.goto(process.argv[2]+'/edit');
   assert.equal(await page.locator('#upload-cv').count(),1);
   await page.locator('#cv-file').setInputFiles({name:'fixture.pdf',mimeType:'application/pdf',buffer:Buffer.from('%PDF-1.4\nsynthetic-'+name)});
   await page.locator('#upload-cv').click();
   await page.waitForFunction(()=>document.querySelector('#cv-status').textContent==='CV uploaded');
   await page.locator('#cv-file').scrollIntoViewIfNeeded();
   for (const selector of ['#cv-status', '#cv-file', '#upload-cv']) {
    const box=await page.locator(selector).boundingBox(); assert.ok(box && box.x>=0 && box.x+box.width<=width, `${name}: ${selector} overflows`);
   }
   if (process.env.CV_SCREENSHOT_DIR) await page.screenshot({path:require('node:path').join(process.env.CV_SCREENSHOT_DIR, `${name}.png`)});
   await page.goto(process.argv[2]+'/profile');assert.equal(await page.getByText('Download CV',{exact:true}).count(),0);
   assert.equal((await context.request.get(process.argv[2]+'/x/developer-cv/browser-profile')).status(),403);
   await context.request.get(process.argv[2]+'/fixture/recruiter');await page.goto(process.argv[2]+'/profile');
   const [download]=await Promise.all([page.waitForEvent('download'),page.getByText('Download CV',{exact:true}).click()]);
   assert.equal(download.suggestedFilename(),'cv.pdf');assert.equal(await download.failure(),null);
   assert.deepEqual(errors,[]);
   await context.close();console.log(`${name}: real multipart upload, persisted CV, developer denial and recruiter download PASS`);
  }
 } finally {await browser.close()}
})().catch(e=>{console.error(e);process.exitCode=1});
