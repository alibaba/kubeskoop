export async function submitForm(data: Record<string, any>) {
  // return await request.post('/form/submit', data);
  console.log(data);
  return { data: {}, success: true };
}
